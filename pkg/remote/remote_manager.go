package remote

import (
	"context"
	"fmt"
	u "net/url"
	"os"
	"path/filepath"
	"sync"
	"wails-cast/pkg/extractor"
	"wails-cast/pkg/filehelper"
	"wails-cast/pkg/folders"
	"wails-cast/pkg/hls"
)

type RemoteManager struct {
	mu    sync.Mutex // guards items
	items map[string]*MediaManager
	Cache bool
}

type ExtractionData struct {
	URL         string
	Cookies     map[string]string
	Headers     map[string]string
	Title       string
	ManifestURL string
	// Subtitles are the VTT files the page loaded beside its stream.
	Subtitles []PageSubtitle
}

// PageSubtitle is a subtitle file kept in the page's cache folder.
type PageSubtitle struct {
	Label string
	File  string // name within the page's cache folder
}

// PageSubtitleContent is a subtitle a page loaded, as fetched.
type PageSubtitleContent struct {
	Label   string
	Content string
}

func (m *RemoteManager) GetDownloadStatus(url string, mediaType string, track int) (*DownloadStatusQeuryResponse, error) {
	media, err := m.GetMedia(url)
	if err != nil {
		return nil, err
	}
	status, err := media.GetDownloadStatus(mediaType, track)
	if err != nil {
		return nil, err
	}
	return status, nil
}

func (m *RemoteManager) StopDownload(url string, mediaType string, index int) error {
	media, err := m.GetMedia(url)
	if err != nil {
		return err
	}
	return media.StopDownload(mediaType, index)
}

func (m *RemoteManager) StartDownload(url string, mediaType string, index int) error {
	media, err := m.GetMedia(url)
	if err != nil {
		return err
	}
	return media.StartDownload(mediaType, index)
}

// NewManager creates a new remote manager
func NewManager(cache bool) *RemoteManager {
	return &RemoteManager{
		items: make(map[string]*MediaManager),
		Cache: cache,
	}
}

func (m *RemoteManager) StopAllAndClear() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, item := range m.items {
		err := item.StopAllAndClear()
		if err != nil {
			return err
		}
	}
	return nil
}

func (m *RemoteManager) GetMedia(url string) (*MediaManager, error) {
	parsed, err := u.Parse(url)
	if err != nil {
		return nil, err
	}
	m.mu.Lock()
	item, exists := m.items[url]
	m.mu.Unlock()
	if exists {
		return item, nil
	}
	extractionFile := extractionFile(url)
	extractionData, err := filehelper.ReadJson[ExtractionData](extractionFile)

	var manifest *hls.ManifestPlaylist
	if err != nil {
		extractionData, manifest, err = m.doExtraction(parsed)
		if err != nil {
			return nil, err
		}
	}

	if err != nil {
		return nil, err
	}

	if manifest == nil {
		bytes, err := os.ReadFile(hlsFile(url))
		if err != nil {
			return nil, err
		}
		manifest, err = hls.ParseManifest(string(bytes))
		if err != nil {
			return nil, err
		}
	}

	manifestUrl, err := u.Parse(extractionData.ManifestURL)
	if err != nil {
		return nil, err
	}
	mediaItem := NewMediaManager(
		url,
		folders.Video(url),
		extractionData.Title,
		manifestUrl,
		manifest,
		&FileDownloader{
			Cookies: extractionData.Cookies,
			Headers: extractionData.Headers,
		},
		m.Cache,
	)
	m.mu.Lock()
	defer m.mu.Unlock()
	if item, exists := m.items[url]; exists {
		return item, nil
	}
	m.items[url] = mediaItem
	return mediaItem, nil
}

// HasExtraction reports whether the page's stream is known without
// extracting it again.
func (m *RemoteManager) HasExtraction(pageURL string) bool {
	m.mu.Lock()
	_, exists := m.items[pageURL]
	m.mu.Unlock()
	return exists || filehelper.Exists(extractionFile(pageURL))
}

// PageSubtitlePath is where a page's subtitle file is kept.
func PageSubtitlePath(pageURL string, sub PageSubtitle) string {
	return filepath.Join(folders.Video(pageURL), sub.File)
}

// CachedExtraction returns the page's stored extraction and its manifest.
func (m *RemoteManager) CachedExtraction(pageURL string) (*ExtractionData, string, error) {
	data, err := filehelper.ReadJson[ExtractionData](extractionFile(pageURL))
	if err != nil {
		return nil, "", err
	}
	manifest, err := os.ReadFile(hlsFile(pageURL))
	if err != nil {
		return nil, "", err
	}
	return data, string(manifest), nil
}

// Seed stores an extraction made elsewhere (another instance's browser) as
// if this instance had extracted the page itself.
func (m *RemoteManager) Seed(data ExtractionData, manifest string, subtitles []PageSubtitleContent) error {
	if _, err := u.Parse(data.ManifestURL); err != nil || data.ManifestURL == "" {
		return fmt.Errorf("invalid manifest URL %q", data.ManifestURL)
	}
	if _, err := hls.ParseManifest(manifest); err != nil {
		return fmt.Errorf("invalid manifest: %w", err)
	}
	// Track playlists cached from an older extraction hold its expired links.
	if old, err := filehelper.ReadJson[ExtractionData](extractionFile(data.URL)); err == nil && old.ManifestURL != data.ManifestURL {
		if err := m.forget(data.URL); err != nil {
			return err
		}
	}
	return storeExtraction(&data, manifest, subtitles)
}

// DropIfStale checks a page's cached stream before casting it, for the
// tracks the cast would play. When it no longer loads, the page's cache is
// dropped, so the next cast extracts it again, and the reason is returned.
func (m *RemoteManager) DropIfStale(ctx context.Context, pageURL string, videoTrack, audioTrack int) (dropped bool, reason error) {
	if !m.HasExtraction(pageURL) {
		return false, nil
	}
	media, err := m.GetMedia(pageURL)
	if err == nil {
		err = media.Probe(ctx, videoTrack, audioTrack)
	}
	if err == nil {
		return false, nil
	}
	if ferr := m.forget(pageURL); ferr != nil {
		return false, ferr
	}
	return true, err
}

// forget stops the page's downloads and removes everything cached for it.
func (m *RemoteManager) forget(pageURL string) error {
	m.mu.Lock()
	item, exists := m.items[pageURL]
	delete(m.items, pageURL)
	m.mu.Unlock()
	if exists {
		for _, track := range item.Items {
			track.StopDownload()
		}
	}
	return os.RemoveAll(folders.Video(pageURL))
}

// storeExtraction writes the per-page cache that GetMedia reads, with the
// page's subtitle files.
func storeExtraction(data *ExtractionData, manifest string, subtitles []PageSubtitleContent) error {
	if err := filehelper.WriteFile(hlsFile(data.URL), []byte(manifest)); err != nil {
		return err
	}
	data.Subtitles = nil
	for i, sub := range subtitles {
		stored := PageSubtitle{Label: sub.Label, File: fmt.Sprintf("page_subtitle_%d.vtt", i)}
		if err := filehelper.WriteFile(PageSubtitlePath(data.URL, stored), []byte(sub.Content)); err != nil {
			return err
		}
		data.Subtitles = append(data.Subtitles, stored)
	}
	return filehelper.WriteJson(extractionFile(data.URL), data)
}

func extractionFile(url string) string {
	return filepath.Join(folders.Video(url), "extraction.json")
}

func (*RemoteManager) doExtraction(url *u.URL) (*ExtractionData, *hls.ManifestPlaylist, error) {
	extraction, err := extractor.Extract(url.String(), nil)
	if err != nil {
		return nil, nil, err
	}

	manifest, err := hls.ParseManifest(extraction.Manifest)
	if err != nil {
		return nil, nil, err
	}
	extractionData := &ExtractionData{
		URL:         url.String(),
		Cookies:     extraction.Cookies,
		Headers:     extraction.Headers,
		Title:       extraction.Title,
		ManifestURL: extraction.ManifestURL,
	}
	subtitles := make([]PageSubtitleContent, 0, len(extraction.Subtitles))
	for _, sub := range extraction.Subtitles {
		subtitles = append(subtitles, PageSubtitleContent{Label: sub.Label, Content: sub.Content})
	}
	if err := storeExtraction(extractionData, extraction.Manifest, subtitles); err != nil {
		return nil, nil, err
	}
	return extractionData, manifest, nil
}

func hlsFile(url string) string {
	videoFolder := folders.Video(url)
	hlsFile := filepath.Join(videoFolder, "playlist.m3u8")
	return hlsFile
}
