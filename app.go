package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vishen/go-chromecast/application"
	cast_proto "github.com/vishen/go-chromecast/cast/proto"
	wails_runtime "github.com/wailsapp/wails/v2/pkg/runtime"

	"frog-cast/pkg/ai"
	"frog-cast/pkg/castapi"
	"frog-cast/pkg/events"
	"frog-cast/pkg/extractor"
	"frog-cast/pkg/ffmpeg"
	"frog-cast/pkg/folders"
	"frog-cast/pkg/hls"
	"frog-cast/pkg/linq"
	_logger "frog-cast/pkg/logger"
	"frog-cast/pkg/options"
	"frog-cast/pkg/remote"
	"frog-cast/pkg/stream"
	"frog-cast/pkg/webextract"
)

var logger = _logger.Logger
var customAppID = "7B88BB2E" // Custom receiver app ID

// errNoWindow: a dialog was asked of a headless instance.
var errNoWindow = errors.New("no window: this instance runs headless")

type App struct {
	ctx context.Context
	// hasWindow is false in a headless instance: no dialogs, no frontend.
	hasWindow bool
	App       castReceiver
	// newReceiver connects to a cast device; tests replace it with a fake.
	newReceiver func(*App) castReceiver
	discovery   *DeviceDiscovery
	mediaServer *Server
	httpServer  *HTTPServer // remote control API for companion apps
	// localIp is this machine's address toward the current cast's device,
	// set when the cast starts (the network can change after launch).
	localIp       string
	port          int
	playbackState PlaybackState
	// positionAt is when playbackState.CurrentTime was last set. The device
	// only reports its position on status changes, so remote clients get the
	// position extrapolated from here (see currentPlaybackState).
	positionAt time.Time
	// streamOptions is what the current cast plays: tracks, quality and
	// subtitle source and style. Live changes update it; /state reports it.
	// nil until the first cast.
	streamOptions *options.StreamOptions
	historyStore  *HistoryStore
	settingsStore *SettingsStore
	mu            sync.RWMutex
	RemoteManager *remote.RemoteManager

	translationCancel context.CancelFunc
	translationMu     sync.Mutex
}

func (a *App) createApplication() {
	a.stopPlayback(false) // ignore error
	newReceiver := a.newReceiver
	if newReceiver == nil {
		newReceiver = newChromecastReceiver
	}
	receiver := newReceiver(a)
	a.mu.Lock()
	a.App = receiver
	a.mu.Unlock()
}

func NewApp() *App {
	port := 8888
	app := &App{
		discovery:     NewDeviceDiscovery(),
		mediaServer:   NewServer(port),
		port:          port,
		playbackState: PlaybackState{Status: "STOPPED"},
		historyStore:  NewHistoryStore(),
		settingsStore: NewSettingsStore(),
		RemoteManager: remote.NewManager(true),
	}
	app.httpServer = NewHTTPServer(app)
	return app
}

// startup runs when the Wails window opens: frontend events and the WebKit
// loop come from the window, then the services start.
func (a *App) startup(ctx context.Context) {
	a.hasWindow = true
	events.Subscribe(func(topic string, payload any) {
		wails_runtime.EventsEmit(ctx, topic, payload)
	})
	webextract.UseHostLoop()
	if err := a.startServices(ctx); err != nil {
		logger.Error("Failed to start media server", "error", err)
	}
	if a.settingsStore.Get().RemoteAPIEnabled {
		if err := a.startRemoteAPI(); err != nil {
			logger.Error("Failed to start remote API", "error", err)
		}
	}
	logger.Info("App started")
}

// serveHeadless starts everything remote clients use, with no window: the
// remote API always runs, whatever the setting, and a failed bind is fatal.
func (a *App) serveHeadless(ctx context.Context) error {
	if err := a.startServices(ctx); err != nil {
		return err
	}
	if err := a.startRemoteAPI(); err != nil {
		return err
	}
	logger.Info("Headless instance serving", "remoteApiPort", a.settingsStore.Get().RemoteAPIPort, "mediaPort", a.port)
	return nil
}

// startServices starts what both modes share; ctx bounds long-running work.
func (a *App) startServices(ctx context.Context) error {
	a.ctx = ctx
	// Wire the library scanner in so /library serves real items instead of
	// the history fallback.
	a.httpServer.SetLibraryLister(a)
	return a.mediaServer.Start()
}

func (a *App) startRemoteAPI() error {
	return a.httpServer.Start(a.settingsStore.Get().RemoteAPIPort)
}

func (a *App) shutdown(ctx context.Context) {
	a.mediaServer.Stop()
	a.httpServer.Stop()
}

// DiscoverDevices discovers Chromecast devices on the network
func (a *App) DiscoverDevices() []Device {
	// Start streaming discovery and emit events for each device found
	err := a.discovery.DiscoverStream()
	if err != nil {
		logger.Error("Device discovery failed to start", "error", err)
	}
	// Return immediately; frontend will receive devices via events
	return []Device{}
}

// streamHost is the address the cast device fetches the stream from.
func (a *App) streamHost() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.localIp
}

// getMediaURL returns the URL for a media file to be cast
func (a *App) getMediaURL() string {
	return fmt.Sprintf("http://%s:%d/playlist.m3u8", a.streamHost(), a.port)
}

func (a *App) GetTrackDisplayInfo(fileNameOrUrl string) (*TrackDisplayInfo, error) {
	trackInfo := &hls.ManifestPlaylist{}
	var err error
	var pageSubtitles []SubtitleDisplayItem
	webPage := isWebURL(fileNameOrUrl)
	if webPage {
		mediaManager, err := a.RemoteManager.GetMedia(fileNameOrUrl)
		if err != nil {
			return nil, err
		}
		trackInfo = mediaManager.Manifest
		if data, _, err := a.RemoteManager.CachedExtraction(fileNameOrUrl); err == nil {
			for _, sub := range data.Subtitles {
				pageSubtitles = append(pageSubtitles, SubtitleDisplayItem{
					Path:  "external:" + remote.PageSubtitlePath(fileNameOrUrl, sub),
					Label: "Page: " + sub.Label,
				})
			}
		}
	} else {
		trackInfo, err = ffmpeg.GetMediaTrackInfo(fileNameOrUrl)
	}
	if err != nil {
		return nil, err
	}
	var subtitleItems = []SubtitleDisplayItem{
		{
			Path:  "none",
			Label: "None",
		},
		{
			Path:  "external",
			Label: "External",
		},
	}

	for _, sub := range trackInfo.SubtitleTracks {
		subtitleItems = append(subtitleItems, SubtitleDisplayItem{
			Path:  fmt.Sprintf("embedded:%d", sub.Index),
			Label: fmt.Sprintf("Embedded: %s", sub.Name),
		})
	}
	subtitleItems = append(subtitleItems, pageSubtitles...)

	nearSubtitle := ""
	if !webPage {
		// Prefer an existing translation for the default target language so it
		// is auto-selected; otherwise fall back to a sibling subtitle file.
		lang := a.settingsStore.Get().DefaultTranslationLanguage
		if hasTranslation(fileNameOrUrl, lang) {
			nearSubtitle = translatedSubtitlePath(fileNameOrUrl, lang)
		} else {
			nearSubtitle = findSubtitleFile(fileNameOrUrl)
		}
	}

	return &TrackDisplayInfo{
		VideoTracks: linq.Map(trackInfo.VideoTracks, func(v hls.VideoTrack) VideoTrackDisplayItem {
			return VideoTrackDisplayItem{
				Index:      v.Index,
				Resolution: v.Resolution,
				Codecs:     v.Codecs,
			}
		}),
		AudioTracks: linq.Map(trackInfo.AudioTracks, func(a hls.AudioTrack) AudioTracksDisplayItem {
			return AudioTracksDisplayItem{
				Index:    a.Index,
				Language: a.Label(),
			}
		}),
		SubtitleTracks: subtitleItems,
		Path:           fileNameOrUrl,
		NearSubtitle:   nearSubtitle,
	}, nil
}

// CastToDevice casts media (local file or remote URL) to a device, with this
// instance's subtitle style.
func (a *App) CastToDevice(deviceIp string, fileNameOrUrl string, castOptions *options.CastOptions) (*PlaybackState, error) {
	return a.castWithStyle(deviceIp, fileNameOrUrl, castOptions, nil, true)
}

// readyPage checks a web page's cached stream before it is cast, dropping it
// when its links no longer load. Without a browser on this screen
// (browserHere false), a page that would then need one is refused with
// castapi.ErrNeedsExtraction.
func (a *App) readyPage(pageURL string, videoTrack, audioTrack int, browserHere bool) error {
	dropped, reason := a.RemoteManager.DropIfStale(a.ctx, pageURL, videoTrack, audioTrack)
	if !dropped && reason != nil {
		return reason
	}
	if dropped {
		logger.Warn("Cached stream no longer loads, extracting again", "url", pageURL, "reason", reason)
	}
	if browserHere || a.RemoteManager.HasExtraction(pageURL) || (extractor.HasScript(pageURL) && webextract.Available()) {
		return nil
	}
	if dropped {
		return fmt.Errorf("the page's stream link expired (%v); send it again from the desktop app: %w", reason, castapi.ErrNeedsExtraction)
	}
	return fmt.Errorf("this instance can't extract the page (no site script, or no browser engine when headless): send it from the desktop app: %w", castapi.ErrNeedsExtraction)
}

// castWithStyle casts with the subtitle style of the controller that started
// the cast; nil uses this instance's settings. The timing offset is always
// the one this instance keeps for the media. browserHere is whether a web
// page may be extracted in a browser on this screen.
func (a *App) castWithStyle(deviceIp, fileNameOrUrl string, castOptions *options.CastOptions, style *castapi.SubtitleStyle, browserHere bool) (*PlaybackState, error) {
	if isWebURL(fileNameOrUrl) {
		if err := a.readyPage(fileNameOrUrl, castOptions.VideoTrack, castOptions.AudioTrack, browserHere); err != nil {
			return nil, err
		}
	}
	opts := a.streamOptionsFor(castOptions, style)
	opts.Subtitle.DelaySeconds = a.historyStore.SubtitleDelay(fileNameOrUrl)
	state, err := a.startCast(deviceIp, fileNameOrUrl, opts, 0, false)
	if err != nil {
		return nil, err
	}
	a.historyStore.Add(fileNameOrUrl, state.MediaName, castOptions)
	return state, nil
}

// streamOptionsFor combines a cast's track choice and subtitle style with
// this instance's transcode settings. A nil style uses this instance's.
func (a *App) streamOptionsFor(castOptions *options.CastOptions, style *castapi.SubtitleStyle) options.StreamOptions {
	settings := a.GetSettings()
	if style == nil {
		style = &castapi.SubtitleStyle{
			FontSize: settings.SubtitleFontSize,
			Bold:     settings.SubtitleBold,
			Italic:   settings.SubtitleItalic,
			BurnIn:   settings.SubtitleBurnIn,
		}
	}
	return options.StreamOptions{
		Subtitle: options.SubtitleCastOptions{
			Path:                 castOptions.SubtitlePath,
			BurnIn:               style.BurnIn,
			FontSize:             style.FontSize,
			IgnoreClosedCaptions: settings.IgnoreClosedCaptions,
			Bold:                 style.Bold,
			Italic:               style.Italic,
		},
		VideoTrack:       castOptions.VideoTrack,
		AudioTrack:       castOptions.AudioTrack,
		Bitrate:          castOptions.Bitrate,
		MaxOutputWidth:   settings.MaxOutputWidth,
		NoTranscodeCache: settings.NoTranscodeCache,
	}
}

// prepareStream builds the stream handler for the media and installs it on
// the media server. It returns the media's display name and duration.
func (a *App) prepareStream(fileNameOrUrl string, opts options.StreamOptions) (string, float64, error) {
	if isWebURL(fileNameOrUrl) {
		logger.Info("Preparing remote stream", "url", fileNameOrUrl)
		manager, err := a.RemoteManager.GetMedia(fileNameOrUrl)
		if err != nil {
			return "", 0, fmt.Errorf("failed to prepare remote stream: %w", err)
		}
		handler, err := stream.NewRemoteHandler(a.ctx, manager, opts, folders.Video(fileNameOrUrl))
		if err != nil {
			return "", 0, fmt.Errorf("failed to create remote handler: %w", err)
		}
		a.mediaServer.SetHandler(handler)
		return manager.Title, manager.GetDuration(), nil
	}

	duration, err := ffmpeg.GetVideoDuration(fileNameOrUrl)
	if err != nil {
		logger.Warn("Failed to get duration", "error", err)
		duration = 0
	}
	a.mediaServer.SetHandler(stream.NewLocalHandler(fileNameOrUrl, opts))
	a.mediaServer.SetSubtitlePath(opts.Subtitle.Path)
	return filepath.Base(fileNameOrUrl), duration, nil
}

// startCast serves the media with opts and loads it on the device at start
// seconds, playing or paused. deviceIp "local" only hosts the stream. While
// the device loads, the status is LOADING; the device's own reports end it.
func (a *App) startCast(deviceIp, fileNameOrUrl string, opts options.StreamOptions, start float64, paused bool) (*PlaybackState, error) {
	name, duration, err := a.prepareStream(fileNameOrUrl, opts)
	if err != nil {
		return nil, err
	}

	hostOnly := deviceIp == "local"
	toward := deviceIp
	if hostOnly {
		toward = ""
	}
	localIP := a.discovery.LocalIPFor(toward)
	a.mu.Lock()
	a.localIp = localIP
	a.playbackState.MediaPath = fileNameOrUrl
	a.playbackState.MediaName = name
	a.playbackState.DeviceURL = deviceIp
	a.playbackState.DeviceName = extractDeviceName(deviceIp)
	a.playbackState.Duration = duration
	a.playbackState.CurrentTime = start
	a.positionAt = time.Now()
	a.streamOptions = &opts
	switch {
	case !hostOnly:
		a.playbackState.Status = "LOADING"
	case paused:
		a.playbackState.Status = "PAUSED"
	default:
		a.playbackState.Status = "PLAYING"
	}
	state := a.stateLocked()
	a.mu.Unlock()
	events.Emit("playback:state", state)

	if hostOnly {
		logger.Info("Hosting stream without casting", "url", a.getMediaURL())
		return &state, nil
	}

	if err := a.loadOnDevice(deviceIp, start, paused, opts); err != nil {
		a.mu.Lock()
		a.playbackState.Status = "STOPPED"
		state := a.stateLocked()
		a.mu.Unlock()
		events.Emit("playback:state", state)
		return nil, err
	}

	logger.Info("Cast successful",
		"message", fmt.Sprintf("Casting %s to %s via %s", name, deviceIp, a.getMediaURL()),
		"device", deviceIp,
		"media", fileNameOrUrl,
		"start", start,
		"subtitle", opts.Subtitle.Path,
	)
	a.mu.RLock()
	state = a.stateLocked()
	a.mu.RUnlock()
	return &state, nil
}

// loadOnDevice connects to the device and loads the hosted stream on it.
func (a *App) loadOnDevice(deviceIp string, start float64, paused bool, opts options.StreamOptions) error {
	a.createApplication()
	if err := a.App.Start(deviceIp, 8009); err != nil {
		return err
	}
	err := a.App.Load(a.getMediaURL()+"?cachebust="+time.Now().Format("20060102150405"), application.LoadOptions{
		StartTime:   int(start),
		Paused:      paused,
		Transcode:   false,
		Detach:      true,
		ForceDetach: false,
		ContentType: "application/vnd.apple.mpegurl",
		AppId:       customAppID,
	})
	if err != nil {
		return fmt.Errorf("failed to load media on device: %w", err)
	}
	if err := a.App.Update(); err != nil {
		return fmt.Errorf("failed to update media status: %w", err)
	}
	if opts.Subtitle.Path != "none" && !opts.Subtitle.BurnIn {
		a.sendSubtitles(fmt.Sprintf("http://%s:%d/subtitles.vtt", a.streamHost(), a.port))
	}
	return nil
}

// ChangeTracks switches the running cast's video track, audio track, quality
// or subtitle source; nil fields keep the current choice. An external
// subtitle change without burn-in is applied live. Anything else re-casts the
// same media to the same device at the current position, keeping it paused
// if it was.
func (a *App) ChangeTracks(change castapi.TrackChange) (*PlaybackState, error) {
	a.mu.RLock()
	if a.streamOptions == nil || a.playbackState.Status == "STOPPED" {
		a.mu.RUnlock()
		return nil, fmt.Errorf("nothing is playing")
	}
	current := *a.streamOptions
	mediaPath := a.playbackState.MediaPath
	deviceIp := a.playbackState.DeviceURL
	paused := a.playbackState.Status == "PAUSED"
	position := a.extrapolatedPosition()
	state := a.stateLocked()
	a.mu.RUnlock()

	next := current
	if change.VideoTrack != nil {
		next.VideoTrack = *change.VideoTrack
	}
	if change.AudioTrack != nil {
		next.AudioTrack = *change.AudioTrack
	}
	if change.Quality != nil {
		next.Bitrate = *change.Quality
	}
	if change.SubtitlePath != nil {
		next.Subtitle.Path = *change.SubtitlePath
		if next.Subtitle.Path == "" {
			next.Subtitle.Path = "none"
		}
	}
	if next == current {
		return &state, nil
	}

	sameStream := next.VideoTrack == current.VideoTrack &&
		next.AudioTrack == current.AudioTrack &&
		next.Bitrate == current.Bitrate
	if sameStream && !next.Subtitle.BurnIn {
		if err := a.UpdateSubtitleSettings(next.Subtitle); err != nil {
			return nil, err
		}
		s := a.currentPlaybackState()
		state = s
	} else {
		logger.Info("Re-casting with new tracks", "media", mediaPath, "position", position)
		s, err := a.startCast(deviceIp, mediaPath, next, position, paused)
		if err != nil {
			return nil, err
		}
		state = *s
	}

	a.historyStore.Add(mediaPath, state.MediaName, &options.CastOptions{
		SubtitlePath: next.Subtitle.Path,
		VideoTrack:   next.VideoTrack,
		AudioTrack:   next.AudioTrack,
		Bitrate:      next.Bitrate,
	})
	return &state, nil
}

// GetMediaFiles returns media files from a directory
func (a *App) GetMediaFiles(dirPath string) ([]string, error) {
	var files []string
	extensions := map[string]bool{
		".mp4": true, ".mkv": true, ".webm": true,
		".avi": true, ".mov": true, ".flv": true,
		".m4v": true,
	}

	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			ext := strings.ToLower(filepath.Ext(path))
			if extensions[ext] {
				files = append(files, path)
			}
		}
		return nil
	})

	return files, err
}

func findSubtitleFile(videoPath string) string {
	baseName := strings.TrimSuffix(videoPath, filepath.Ext(videoPath))
	exts := []string{".srt", ".vtt", ".ass", ".ssa"}

	for _, ext := range exts {
		subPath := baseName + ext
		if _, err := os.Stat(subPath); err == nil {
			logger.Info("Found subtitle file", "path", subPath)
			return subPath
		}
	}

	return ""
}

// SeekTo seeks to a specific time
func (a *App) SeekTo(seekTime float64) error {
	// Send seek command to Chromecast
	if a.App != nil {
		err := a.App.SeekToTime(float32(seekTime))
		if err != nil {
			logger.Error("Seek failed", "error", err)
			return err
		}
	}

	a.mu.Lock()
	a.playbackState.CurrentTime = seekTime
	a.positionAt = time.Now()
	state := a.stateLocked()
	a.mu.Unlock()
	events.Emit("playback:state", state)

	logger.Info("Seek successful", "time", seekTime)
	return nil
}

// UpdateSubtitleSettings updates subtitle settings for current media without
// recasting. It pushes the new options (path, font size, bold/italic style and
// timing offset) into the live stream handler, clears the transcode cache and
// re-seeks so burn-in changes take effect. For external (non-burn-in)
// subtitles it also re-sends the VTT URL so the receiver swaps in the freshly
// shifted/styled subtitle track.
//
// An empty Path keeps the current cast's subtitle source. The timing offset
// is remembered for the media, for every later cast of it.
func (a *App) UpdateSubtitleSettings(opts options.SubtitleCastOptions) error {
	a.mu.Lock()
	var previous options.SubtitleCastOptions
	if a.streamOptions != nil {
		previous = a.streamOptions.Subtitle
	}
	if opts.Path == "" {
		opts.Path = previous.Path
	}
	if opts.Path == "" {
		opts.Path = "none"
	}
	if a.streamOptions != nil {
		a.streamOptions.Subtitle = opts
	}
	mediaPath := a.playbackState.MediaPath
	a.mu.Unlock()

	if mediaPath != "" && opts.DelaySeconds != previous.DelaySeconds {
		if err := a.historyStore.SetSubtitleDelay(mediaPath, opts.DelaySeconds); err != nil {
			logger.Warn("Failed to save subtitle offset", "media", mediaPath, "error", err)
		}
	}

	// Update subtitle path + full options on the live handler (clears cache).
	a.mediaServer.SetSubtitlePath(opts.Path)
	a.mediaServer.UpdateSubtitleOptions(opts)

	// Hosting without a cast device: nothing to tell a receiver.
	if a.App == nil {
		return nil
	}
	a.App.Update()

	// For external subtitles, re-send the VTT so the player reloads it with the
	// new timing offset / style applied. The receiver auto-replaces the track.
	if opts.Path != "none" && !opts.BurnIn {
		if opts.FontSize != previous.FontSize {
			a.setSubtitleSize(opts.FontSize)
		}
		a.sendSubtitles(fmt.Sprintf("http://%s:%d/subtitles.vtt", a.streamHost(), a.port))
	}

	// Re-seek to where the TV is now, not to its last report, which can be
	// minutes old and would jump the video back.
	a.mu.RLock()
	position := a.extrapolatedPosition()
	state := a.stateLocked()
	a.mu.RUnlock()
	events.Emit("playback:state", state)
	return a.App.SeekToTime(float32(position))
}

// ClearCache removes the whole media cache, stopping downloads into it first.
func (a *App) ClearCache() error {
	if err := a.RemoteManager.StopAllAndClear(); err != nil {
		return err
	}
	return folders.DeleteAllCache()
}

// GetCacheStats reports the media cache's size by part.
func (a *App) GetCacheStats() (*castapi.CacheStats, error) {
	stats, err := folders.GetCacheStats()
	if err != nil {
		return nil, err
	}
	return &castapi.CacheStats{
		Total: stats.TotalSize, Transcoded: stats.TranscodedSize, RawSegments: stats.RawSegmentsSize, Metadata: stats.MetadataSize,
	}, nil
}

// Downloads lists the web page tracks downloaded or downloading since start.
func (a *App) Downloads() []castapi.PageDownload {
	items := []castapi.PageDownload{}
	for _, d := range a.RemoteManager.Downloads() {
		items = append(items, castapi.PageDownload{
			URL: d.URL, Title: d.Title, Type: d.Type, Track: d.Track,
			DownloadStatus: downloadStatus(&remote.DownloadStatusQeuryResponse{Status: d.Status, Segments: d.Segments}),
		})
	}
	return items
}

// DeleteTranscodedCache removes only transcoded video segments
func (a *App) DeleteTranscodedCache() error {
	if err := a.RemoteManager.StopAllAndClear(); err != nil {
		return err
	}
	return folders.DeleteTranscodedCache()
}

// DeleteAllVideoCache removes all video files but keeps metadata
func (a *App) DeleteAllVideoCache() error {
	if err := a.RemoteManager.StopAllAndClear(); err != nil {
		return err
	}
	return folders.DeleteAllVideoCache()
}

// OpenMediaFolder opens the folder containing the media file
func (a *App) OpenMediaFolder(fileNameOrUrl string) error {
	dir := folders.Video(fileNameOrUrl)
	os.MkdirAll(dir, 0755)
	cmd := exec.Command("open", dir)
	return cmd.Start()
}

func (a *App) Export() AppExports {
	panic("DO NOT CALL")
}

// SetDownload starts or stops downloading one track of a web page into the
// cache, and reports it.
func (a *App) SetDownload(req castapi.DownloadRequest) (*castapi.DownloadStatus, error) {
	var err error
	if req.Start {
		err = a.RemoteManager.StartDownload(req.URL, req.Type, req.Track)
	} else {
		err = a.RemoteManager.StopDownload(req.URL, req.Type, req.Track)
	}
	if err != nil {
		return nil, err
	}
	return a.Download(req.URL, req.Type, req.Track)
}

// Download reports one web page track's download into the cache.
func (a *App) Download(pageURL, kind string, track int) (*castapi.DownloadStatus, error) {
	status, err := a.RemoteManager.GetDownloadStatus(pageURL, kind, track)
	if err != nil {
		return nil, err
	}
	out := downloadStatus(status)
	return &out, nil
}

// Pause pauses current playback
func (a *App) Pause() error {
	if a.App != nil {
		if err := a.App.Pause(); err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.playbackState.CurrentTime = a.extrapolatedPosition()
	a.positionAt = time.Now()
	a.playbackState.Status = "PAUSED"
	state := a.stateLocked()
	a.mu.Unlock()
	events.Emit("playback:state", state)
	return nil
}

// Unpause resumes current playback
func (a *App) Unpause() error {
	if a.App != nil {
		if err := a.App.Unpause(); err != nil {
			return err
		}
	}
	a.mu.Lock()
	a.positionAt = time.Now()
	a.playbackState.Status = "PLAYING"
	state := a.stateLocked()
	a.mu.Unlock()
	events.Emit("playback:state", state)
	return nil
}

// SetVolume sets the volume of the Chromecast (0.0 to 1.0)
func (a *App) SetVolume(value float32) error {
	if a.App == nil {
		return fmt.Errorf("no application active")
	}
	err := a.App.SetVolume(value)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.playbackState.Volume = float64(value)
	state := a.stateLocked()
	a.mu.Unlock()
	events.Emit("playback:state", state)
	return nil
}

// SetMuted mutes or unmutes the Chromecast
func (a *App) SetMuted(muted bool) error {
	if a.App == nil {
		return fmt.Errorf("no application active")
	}
	err := a.App.SetMuted(muted)
	if err != nil {
		return err
	}
	a.mu.Lock()
	a.playbackState.Muted = muted
	state := a.stateLocked()
	a.mu.Unlock()
	events.Emit("playback:state", state)
	return nil
}

// StopPlayback stops current playback
func (a *App) StopPlayback() error {
	err := a.stopPlayback(true)
	if err == nil {
		a.mu.Lock()
		a.playbackState.Status = "STOPPED"
		state := a.stateLocked()
		a.mu.Unlock()
		events.Emit("playback:state", state)
	}
	return err
}

func (a *App) stopPlayback(stopMedia bool) error {
	a.mu.Lock()
	receiver := a.App
	a.App = nil
	a.mu.Unlock()
	if receiver != nil {
		return receiver.Close(stopMedia)
	}
	return nil
}

// OpenFileDialog opens a file picker dialog
func (a *App) OpenFileDialog(title string, filters []string) (string, error) {
	if title == "" {
		title = "Select File"
	}

	// Convert filters to Wails format
	var wailsFilters []wails_runtime.FileFilter
	if len(filters) > 0 {
		pattern := strings.Join(filters, ";")
		wailsFilters = []wails_runtime.FileFilter{
			{
				DisplayName: "Files",
				Pattern:     pattern,
			},
		}
	}
	if !a.hasWindow {
		return "", errNoWindow
	}

	return wails_runtime.OpenFileDialog(a.ctx, wails_runtime.OpenDialogOptions{
		Title:   title,
		Filters: wailsFilters,
	})
}

// LogInfo logs an info message from frontend
func (a *App) LogInfo(message string, args ...any) {
	logger.Info(message, args...)
}

// LogWarn logs a warning message from frontend
func (a *App) LogWarn(message string, args ...any) {
	logger.Warn(message, args...)
}

// LogError logs an error message from frontend
func (a *App) LogError(message string, args ...any) {
	logger.Error(message, args...)
}

// ExportEmbeddedSubtitles exports all embedded subtitle tracks to WebVTT files
func (a *App) ExportEmbeddedSubtitles(videoPath string) error {
	return ffmpeg.ExportEmbeddedSubtitles(videoPath)
}

// TranslateExportedSubtitles exports embedded subtitles and translates them in the
// background, streaming progress via the "translation:stream" event and signalling
// the outcome via "translation:complete", "translation:error" or
// "translation:cancelled".
func (a *App) TranslateExportedSubtitles(fileNameOrUrl string, targetLanguage string) error {
	settings := a.settingsStore.Get()

	// Resolve API key, model, and base URL based on the selected LLM provider.
	var apiKey, model, baseURL string
	switch settings.LLMProvider {
	case ai.ProviderOpenAICompat:
		apiKey = settings.LLMApiKey
		model = settings.LLMModel
		baseURL = settings.LLMBaseURL
		if apiKey == "" {
			return fmt.Errorf("openai-compat API key is required. Please set it in Settings")
		}
		if baseURL == "" {
			return fmt.Errorf("openai-compat base URL is required. Please set it in Settings")
		}
		if model == "" {
			return fmt.Errorf("openai-compat model is required. Please set it in Settings")
		}
	default:
		// "opencode" (or empty/unrecognised) — use opencode credentials.
		apiKey = settings.LLMApiKey
		if apiKey == "" {
			// Fall back to the user's opencode-go credentials.
			if key, err := ai.LoadOpenCodeAPIKey(); err == nil {
				apiKey = key
			}
		}
		if apiKey == "" {
			return fmt.Errorf("opencode API key is required. Please set it in Settings or in opencode's auth.json")
		}
		model = settings.LLMModel
		baseURL = ai.OpenCodeBaseURL
	}

	if targetLanguage == "" {
		return fmt.Errorf("target language is required")
	}

	a.translationMu.Lock()
	if a.translationCancel != nil {
		a.translationMu.Unlock()
		return fmt.Errorf("a translation is already in progress")
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.translationCancel = cancel
	a.translationMu.Unlock()

	req := ai.Request{
		FileNameOrURL:  fileNameOrUrl,
		TargetLanguage: targetLanguage,
		APIKey:         apiKey,
		Model:          model,
		BaseURL:        baseURL,
		PromptTemplate: settings.TranslatePromptTemplate,
		MaxSamples:     settings.MaxSubtitleSamples,
	}

	go func() {
		defer func() {
			a.translationMu.Lock()
			a.translationCancel = nil
			a.translationMu.Unlock()
			cancel()
		}()

		files, err := ai.TranslateForFile(ctx, req)
		if err != nil {
			if ctx.Err() == context.Canceled {
				events.Emit("translation:cancelled", targetLanguage)
			} else {
				events.Emit("translation:error", err.Error())
			}
			return
		}
		events.Emit("translation:complete", files)
	}()

	return nil
}

// CancelTranslation aborts an in-progress background translation, if any.
func (a *App) CancelTranslation() {
	a.translationMu.Lock()
	cancel := a.translationCancel
	a.translationMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// GenerateTranslationPrompt builds and returns the LLM prompt for subtitles without invoking the model
func (a *App) GenerateTranslationPrompt(fileNameOrUrl string, targetLanguage string) (string, error) {
	settings := a.settingsStore.Get()
	if targetLanguage == "" {
		return "", fmt.Errorf("target language is required")
	}
	req := ai.Request{
		FileNameOrURL:  fileNameOrUrl,
		TargetLanguage: targetLanguage,
		PromptTemplate: settings.TranslatePromptTemplate,
		MaxSamples:     settings.MaxSubtitleSamples,
	}
	return ai.GeneratePromptForFile(req)
}

// ProcessPastedTranslation accepts pasted LLM output, parses and writes translated VTT(s)
func (a *App) ProcessPastedTranslation(fileNameOrUrl string, targetLanguage string, pastedAnswer string) ([]string, error) {
	if targetLanguage == "" {
		return nil, fmt.Errorf("target language is required")
	}

	req := ai.Request{
		FileNameOrURL:  fileNameOrUrl,
		TargetLanguage: targetLanguage,
	}
	return ai.ProcessPastedForFile(a.ctx, req, pastedAnswer)
}

// GetHistory returns all history items
func (a *App) GetHistory() []HistoryItem {
	return a.historyStore.GetAll()
}

// RemoveFromHistory removes an item from history
func (a *App) RemoveFromHistory(path string) error {
	return a.historyStore.Remove(path)
}

// GetSettings returns the current settings
func (a *App) GetSettings() *Settings {
	return a.settingsStore.Get()
}

// UpdateSettings updates the settings
func (a *App) UpdateSettings(settings Settings) error {
	return a.settingsStore.Update(settings)
}

// ResetSettings resets settings to defaults
func (a *App) ResetSettings() (*Settings, error) {
	if err := a.settingsStore.Reset(); err != nil {
		return nil, err
	}
	return a.settingsStore.Get(), nil
}

// UpdateSettings updates the settings and applies remote API changes immediately.
// Overrides the simple delegation to settingsStore so we can start/stop the HTTP
// server without requiring an app restart.
func (a *App) ApplyRemoteAPISettings(enabled bool, port int, token string) error {
	settings := a.settingsStore.Get()
	settings.RemoteAPIEnabled = enabled
	settings.RemoteAPIPort = port
	settings.RemoteAPIToken = token
	if err := a.settingsStore.Update(*settings); err != nil {
		return err
	}

	if enabled {
		// Stop first so we can restart on a potentially different port
		a.httpServer.Stop()
		if err := a.httpServer.Start(port); err != nil {
			return fmt.Errorf("remote API: %w", err)
		}
	} else {
		a.httpServer.Stop()
	}
	return nil
}

// ListModels returns available model IDs for the given provider by querying
// the provider's live models endpoint. No hardcoded model names are returned.
// On error or missing credentials an empty list (or the error) is returned —
// the frontend keeps the currently-saved value selectable in any case.
func (a *App) ListModels(provider string) ([]string, error) {
	settings := a.settingsStore.Get()
	switch provider {
	case ai.ProviderOpenAICompat:
		// GET {LLMBaseURL}/v1/models
		modelsURL := strings.TrimRight(settings.LLMBaseURL, "/") + "/v1/models"
		return fetchModels(modelsURL, settings.LLMApiKey)
	default:
		// "opencode" — GET {OpenCodeBaseURL}/models (base already ends in /v1)
		apiKey := settings.LLMApiKey
		if apiKey == "" {
			if key, err := ai.LoadOpenCodeAPIKey(); err == nil {
				apiKey = key
			}
		}
		modelsURL := ai.OpenCodeBaseURL + "/models"
		return fetchModels(modelsURL, apiKey)
	}
}

// fetchModels calls GET modelsURL with the given Bearer key and returns model IDs
// from an OpenAI-style {"data":[{"id":...}]} response. 5 s timeout.
func fetchModels(modelsURL, apiKey string) ([]string, error) {
	if modelsURL == "" || apiKey == "" {
		return nil, fmt.Errorf("no credentials or URL")
	}

	req, err := http.NewRequest(http.MethodGet, modelsURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %s", resp.Status)
	}

	var result struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	models := make([]string, 0, len(result.Data))
	for _, m := range result.Data {
		if m.ID != "" {
			models = append(models, m.ID)
		}
	}
	return models, nil
}

// GetRemoteAPIAddress returns the listening address of the remote API server,
// or an empty string if it is not running.  The frontend shows this to the user
// so they can connect the Android app.
func (a *App) GetRemoteAPIAddress() string {
	if !a.httpServer.IsRunning() {
		return ""
	}
	settings := a.settingsStore.Get()
	return fmt.Sprintf("http://%s:%d", a.discovery.LocalIPFor(""), settings.RemoteAPIPort)
}

// GetFFmpegInfo returns ffmpeg and ffprobe version information
func (a *App) GetFFmpegInfo(searchAgain bool) (*ffmpeg.FFmpegInfo, error) {
	return ffmpeg.GetFFmpegInfo(searchAgain)
}

// ClearHistory clears all history
func (a *App) ClearHistory() error {
	return a.historyStore.Clear()
}

// handleChromecastMessage handles messages from the device connection from.
// A connection that has since been replaced reports on a session that is
// gone, so its messages are dropped.
func (a *App) handleChromecastMessage(from castReceiver, msg *cast_proto.CastMessage) {
	if msg.PayloadUtf8 == nil {
		return
	}
	a.mu.RLock()
	current := a.App == from
	a.mu.RUnlock()
	if !current {
		return
	}

	messageBytes := []byte(*msg.PayloadUtf8)

	// Check message type
	var msgType struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(messageBytes, &msgType); err != nil {
		return
	}

	// Handle different message types
	switch msgType.Type {
	case "RECEIVER_STATUS":
		var resp struct {
			Status struct {
				Volume struct {
					Level float64 `json:"level"`
					Muted bool    `json:"muted"`
				} `json:"volume"`
			} `json:"status"`
		}
		if err := json.Unmarshal(messageBytes, &resp); err == nil {
			a.mu.Lock()
			a.playbackState.Volume = resp.Status.Volume.Level
			a.playbackState.Muted = resp.Status.Volume.Muted
			a.mu.Unlock()
		}

	case "MEDIA_STATUS":
		var resp struct {
			Status []struct {
				CurrentTime float64 `json:"currentTime"`
				PlayerState string  `json:"playerState"`
				IdleReason  string  `json:"idleReason"`
			} `json:"status"`
		}

		if err := json.Unmarshal(messageBytes, &resp); err == nil && len(resp.Status) > 0 {
			status := resp.Status[0]

			a.mu.Lock()
			// While a load is in flight the device still reports on the media
			// being replaced; its IDLE end must not read as the cast stopping.
			if a.playbackState.Status == "LOADING" && status.PlayerState == "IDLE" {
				a.mu.Unlock()
				return
			}
			a.playbackState.CurrentTime = status.CurrentTime
			a.positionAt = time.Now()

			switch status.PlayerState {
			case "PAUSED":
				a.playbackState.Status = "PAUSED"
			case "PLAYING":
				a.playbackState.Status = "PLAYING"
			case "IDLE":
				if status.IdleReason == "FINISHED" || status.IdleReason == "INTERRUPTED" {
					a.playbackState.Status = "STOPPED"
					a.mu.Unlock()
					return
				}
			}
			a.mu.Unlock()
		}

	case "CLOSE", "LOAD_FAILED":
		a.mu.Lock()
		a.playbackState.Status = "STOPPED"
		a.mu.Unlock()
	}
	a.mu.RLock()
	state := a.stateLocked()
	a.mu.RUnlock()
	events.Emit("playback:state", state)
}

// extrapolatedPosition is the playback position now: the last reported one
// plus the time since, while playing. Callers must hold a.mu.
func (a *App) extrapolatedPosition() float64 {
	pos := a.playbackState.CurrentTime
	if a.playbackState.Status == "PLAYING" && !a.positionAt.IsZero() {
		pos += time.Since(a.positionAt).Seconds()
		if d := a.playbackState.Duration; d > 0 && pos > d {
			pos = d
		}
	}
	return pos
}

// currentPlaybackState returns a snapshot of the playback state with the
// position extrapolated to now, for clients that poll it.
func (a *App) currentPlaybackState() PlaybackState {
	a.mu.RLock()
	defer a.mu.RUnlock()
	state := a.stateLocked()
	state.CurrentTime = a.extrapolatedPosition()
	return state
}

// stateLocked returns a copy of the playback state carrying the current
// cast's selection. Callers must hold a.mu.
func (a *App) stateLocked() PlaybackState {
	state := a.playbackState
	if o := a.streamOptions; o != nil {
		state.Selection = &castapi.CastSelection{
			VideoTrack:   o.VideoTrack,
			AudioTrack:   o.AudioTrack,
			Quality:      o.Bitrate,
			SubtitlePath: o.Subtitle.Path,
			FontSize:     o.Subtitle.FontSize,
			Bold:         o.Subtitle.Bold,
			Italic:       o.Subtitle.Italic,
			DelaySeconds: o.Subtitle.DelaySeconds,
			BurnIn:       o.Subtitle.BurnIn,

			IgnoreClosedCaptions: o.Subtitle.IgnoreClosedCaptions,
		}
	}
	return state
}

// Helper functions

// isWebURL reports whether media is a web page or stream URL rather than a
// local file.
func isWebURL(media string) bool {
	return strings.HasPrefix(media, "http://") || strings.HasPrefix(media, "https://")
}

func extractDeviceName(deviceURL string) string {
	// Extract name from URL or return address
	parts := strings.Split(deviceURL, "/")
	if len(parts) > 2 {
		return parts[2]
	}
	return deviceURL
}
