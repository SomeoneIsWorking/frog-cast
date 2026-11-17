package main

// Remote-client methods let this desktop instance act as a controller for
// *another* frog-cast instance over its remote HTTP API. The actual HTTP
// client and mDNS discovery live in pkg/remote so the Fyne mobile app can
// share them.

import (
	"context"
	"fmt"
	"os"

	"frog-cast/pkg/castapi"
	"frog-cast/pkg/options"
	"frog-cast/pkg/remote"
)

type CastInstance = castapi.CastInstance
type RemoteDevice = castapi.RemoteDevice
type RemotePlayOptions = castapi.PlayOptions
type TrackChange = castapi.TrackChange

func (a *App) DiscoverCastInstances() ([]CastInstance, error) {
	return castapi.Discover(context.Background())
}

func (a *App) RemotePing(base, token string) (bool, error) {
	return castapi.New(base, token).Ping()
}

func (a *App) RemoteLibrary(base, token string) ([]LibraryItem, error) {
	return castapi.New(base, token).Library()
}

func (a *App) RemoteDevices(base, token string) ([]RemoteDevice, error) {
	return castapi.New(base, token).Devices()
}

func (a *App) RemoteState(base, token string) (*PlaybackState, error) {
	return castapi.New(base, token).State()
}

func (a *App) RemotePlay(base, token, id, deviceIp string, opts RemotePlayOptions) (*PlaybackState, error) {
	return castapi.New(base, token).Play(id, deviceIp, opts)
}

func (a *App) RemoteControl(base, token, action string, value float64) (*PlaybackState, error) {
	return castapi.New(base, token).Control(action, value)
}

func (a *App) RemoteTrackInfo(base, token, id string) (*TrackDisplayInfo, error) {
	return castapi.New(base, token).TrackInfo(id)
}

func (a *App) RemoteAddTorrent(base, token, magnet string) error {
	return castapi.New(base, token).AddTorrent(magnet)
}

func (a *App) RemoteTorrents(base, token string) ([]TorrentStatus, error) {
	return castapi.New(base, token).Torrents()
}

func (a *App) RemoteLibraryTree(base, token string) (*LibraryScanResult, error) {
	return castapi.New(base, token).LibraryTree()
}

func (a *App) RemoteIdentify(base, token string, result LibraryScanResult) (*LibraryScanResult, error) {
	return castapi.New(base, token).Identify(result)
}

func (a *App) RemoteOrganizePreview(base, token string, result LibraryScanResult) ([]OrganizeMove, error) {
	return castapi.New(base, token).OrganizePreview(result)
}

func (a *App) RemoteOrganizeExecute(base, token string, plan []OrganizeMove) error {
	return castapi.New(base, token).OrganizeExecute(plan)
}

func (a *App) RemoteTranslateSeason(base, token, showName, seasonName string, episodePaths []string, language string) error {
	return castapi.New(base, token).TranslateSeason(showName, seasonName, episodePaths, language)
}

func (a *App) RemoteSeasonStatus(base, token string) (*SeasonTranslateProgress, error) {
	return castapi.New(base, token).SeasonStatus()
}

func (a *App) RemoteSeasonCancel(base, token string) error {
	return castapi.New(base, token).SeasonCancel()
}

func (a *App) RemoteTranslateFile(base, token, id, language string) error {
	return castapi.New(base, token).TranslateFile(id, language)
}

func (a *App) RemoteTranslateStatus(base, token string) (*translateStatus, error) {
	s, err := castapi.New(base, token).TranslateStatus()
	if err != nil {
		return nil, err
	}
	out := translateStatus(*s)
	return &out, nil
}

func (a *App) RemoteUpdateSubtitle(base, token string, opts options.SubtitleCastOptions) error {
	return castapi.New(base, token).UpdateSubtitle(opts)
}

func (a *App) RemoteChangeTracks(base, token string, change TrackChange) (*PlaybackState, error) {
	return castapi.New(base, token).ChangeTracks(change)
}

// RemotePlayExtracted casts a web page through another instance, extracting
// it here first: in this machine's browser, where someone can click play on a
// site without a script.
func (a *App) RemotePlayExtracted(base, token, pageURL, deviceIp string, opts RemotePlayOptions) (*PlaybackState, error) {
	if !isWebURL(pageURL) {
		return nil, fmt.Errorf("not a web page: %s", pageURL)
	}
	if err := a.readyPage(pageURL, opts.VideoTrack, opts.AudioTrack, true); err != nil {
		return nil, err
	}
	if _, err := a.RemoteManager.GetMedia(pageURL); err != nil {
		return nil, fmt.Errorf("extracting %s: %w", pageURL, err)
	}
	data, manifest, err := a.RemoteManager.CachedExtraction(pageURL)
	if err != nil {
		return nil, err
	}
	req := castapi.PlayExtractedRequest{
		Page: castapi.ExtractedPage{
			PageURL:     pageURL,
			Title:       data.Title,
			ManifestURL: data.ManifestURL,
			Manifest:    manifest,
			Headers:     data.Headers,
			Cookies:     data.Cookies,
		},
		DeviceIP:    deviceIp,
		PlayOptions: opts,
	}
	// The page's subtitle files go along; a picked one is named by index,
	// since the remote keeps its copy at its own path.
	for i, sub := range data.Subtitles {
		path := remote.PageSubtitlePath(pageURL, sub)
		content, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		req.Page.Subtitles = append(req.Page.Subtitles, castapi.PageSubtitle{Label: sub.Label, Content: string(content)})
		if opts.SubtitlePath == "external:"+path {
			req.PageSubtitle = &i
			req.SubtitlePath = ""
		}
	}
	return castapi.New(base, token).PlayExtracted(req)
}
