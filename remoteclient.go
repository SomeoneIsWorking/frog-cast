package main

// Remote-client methods let this desktop instance act as a controller for
// *another* frog-cast instance over its remote HTTP API. The actual HTTP
// client and mDNS discovery live in pkg/remote so the Fyne mobile app can
// share them.

import (
	"context"
	"errors"
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

// RemoteOpenPage returns a web page's tracks as another instance has them,
// to cast it there with RemotePlay: from its own extraction when it has one
// or a script for the page, else from one made here and sent to it.
func (a *App) RemoteOpenPage(base, token, pageURL string) (*TrackDisplayInfo, error) {
	info, err := castapi.New(base, token).TrackInfo(pageURL)
	if errors.Is(err, castapi.ErrNeedsExtraction) {
		return a.sendExtracted(base, token, pageURL)
	}
	return info, err
}

// sendExtracted gives another instance a web page extracted here, in this
// machine's browser where someone can click play on a site without a
// script, and returns the page's tracks as that instance has them.
func (a *App) sendExtracted(base, token, pageURL string) (*TrackDisplayInfo, error) {
	if !isWebURL(pageURL) {
		return nil, fmt.Errorf("not a web page: %s", pageURL)
	}
	if err := a.readyPage(pageURL, 0, 0, true); err != nil {
		return nil, err
	}
	if _, err := a.RemoteManager.GetMedia(pageURL); err != nil {
		return nil, fmt.Errorf("extracting %s: %w", pageURL, err)
	}
	data, manifest, err := a.RemoteManager.CachedExtraction(pageURL)
	if err != nil {
		return nil, err
	}
	page := castapi.ExtractedPage{
		PageURL:     pageURL,
		Title:       data.Title,
		ManifestURL: data.ManifestURL,
		Manifest:    manifest,
		Headers:     data.Headers,
		Cookies:     data.Cookies,
	}
	for _, sub := range data.Subtitles {
		content, err := os.ReadFile(remote.PageSubtitlePath(pageURL, sub))
		if err != nil {
			return nil, err
		}
		page.Subtitles = append(page.Subtitles, castapi.PageSubtitle{Label: sub.Label, Content: string(content)})
	}
	return castapi.New(base, token).SendExtracted(page)
}

// RemoteSetDownload starts or stops downloading a web page track on another
// instance.
func (a *App) RemoteSetDownload(base, token string, req castapi.DownloadRequest) (*castapi.DownloadStatus, error) {
	return castapi.New(base, token).SetDownload(req)
}

// RemoteDownload reports a web page track's download on another instance.
func (a *App) RemoteDownload(base, token, pageURL, kind string, track int) (*castapi.DownloadStatus, error) {
	return castapi.New(base, token).Download(pageURL, kind, track)
}

// RemoteDownloads lists the web page tracks another instance has downloaded
// or is downloading.
func (a *App) RemoteDownloads(base, token string) ([]castapi.PageDownload, error) {
	return castapi.New(base, token).Downloads()
}

// RemoteHistory lists what another instance has cast, newest first.
func (a *App) RemoteHistory(base, token string) ([]castapi.HistoryItem, error) {
	return castapi.New(base, token).History()
}

// RemoteRemoveHistory removes one entry from another instance's history; ""
// clears it.
func (a *App) RemoteRemoveHistory(base, token, path string) error {
	return castapi.New(base, token).RemoveHistory(path)
}

// RemoteCacheStats reports another instance's media cache.
func (a *App) RemoteCacheStats(base, token string) (*castapi.CacheStats, error) {
	return castapi.New(base, token).CacheStats()
}

// RemoteClearCache clears part of another instance's media cache.
func (a *App) RemoteClearCache(base, token, part string) (*castapi.CacheStats, error) {
	return castapi.New(base, token).ClearCache(part)
}
