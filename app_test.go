package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/vishen/go-chromecast/application"

	"wails-cast/pkg/folders"
	"wails-cast/pkg/options"
	"wails-cast/pkg/remote"
)

// TestMain points the config and cache directories at a temp dir so tests
// never touch the user's history, settings or transcode cache.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "wails-cast-test")
	if err != nil {
		panic(err)
	}
	os.Setenv("HOME", dir)
	os.Setenv("XDG_CONFIG_HOME", dir)
	os.Setenv("TMPDIR", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// fakeReceiver records what App asks of the TV.
type fakeReceiver struct {
	mu      sync.Mutex
	loads   []application.LoadOptions
	seeks   []float32
	paused  int
	custom  []string
	closed  int
	started int
}

func (f *fakeReceiver) Start(string, int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started++
	return nil
}

func (f *fakeReceiver) Load(_ string, opts application.LoadOptions) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.loads = append(f.loads, opts)
	return nil
}

func (f *fakeReceiver) Update() error { return nil }

func (f *fakeReceiver) SeekToTime(value float32) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seeks = append(f.seeks, value)
	return nil
}

func (f *fakeReceiver) Pause() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.paused++
	return nil
}

func (f *fakeReceiver) Unpause() error          { return nil }
func (f *fakeReceiver) SetVolume(float32) error { return nil }
func (f *fakeReceiver) SetMuted(bool) error     { return nil }
func (f *fakeReceiver) Close(bool) error        { f.closed++; return nil }
func (f *fakeReceiver) SendCustom(_ string, typ string, _ any) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.custom = append(f.custom, typ)
	return nil
}

// newTestApp builds an App whose cast device is a fake receiver. The media
// server is never started.
func newTestApp(t *testing.T) (*App, *fakeReceiver) {
	t.Helper()
	fake := &fakeReceiver{}
	a := &App{
		mediaServer:   NewServer("127.0.0.1", 0),
		localIp:       "127.0.0.1",
		historyStore:  NewHistoryStore(),
		settingsStore: NewSettingsStore(),
		newReceiver:   func(*App) castReceiver { return fake },
	}
	a.App = fake
	return a, fake
}

func TestUpdateSubtitleSettingsEmptyPathKeepsSource(t *testing.T) {
	a, fake := newTestApp(t)
	a.streamOptions = &options.StreamOptions{Subtitle: options.SubtitleCastOptions{Path: "external:/media/show.srt"}}

	if err := a.UpdateSubtitleSettings(options.SubtitleCastOptions{FontSize: 30}); err != nil {
		t.Fatal(err)
	}
	if got := a.streamOptions.Subtitle.Path; got != "external:/media/show.srt" {
		t.Fatalf("subtitle source changed to %q", got)
	}
	if got := a.streamOptions.Subtitle.FontSize; got != 30 {
		t.Fatalf("font size = %d, want 30", got)
	}
	if len(fake.custom) != 1 || fake.custom[0] != "subtitles" {
		t.Fatalf("external subtitles not re-sent: %v", fake.custom)
	}
}

func TestUpdateSubtitleSettingsSeeksToExtrapolatedPosition(t *testing.T) {
	a, fake := newTestApp(t)
	a.streamOptions = &options.StreamOptions{Subtitle: options.SubtitleCastOptions{Path: "none"}}
	a.playbackState = PlaybackState{Status: "PLAYING", CurrentTime: 100, Duration: 3600}
	a.positionAt = time.Now().Add(-90 * time.Second)

	if err := a.UpdateSubtitleSettings(options.SubtitleCastOptions{}); err != nil {
		t.Fatal(err)
	}
	if len(fake.seeks) != 1 {
		t.Fatalf("seeks = %v", fake.seeks)
	}
	if got := fake.seeks[0]; got < 189 || got > 192 {
		t.Fatalf("re-seeked to %v, want about 190", got)
	}
}

func TestStateReportsSelection(t *testing.T) {
	a, _ := newTestApp(t)
	a.playbackState = PlaybackState{Status: "PAUSED", MediaPath: "/media/film.mkv"}
	a.streamOptions = &options.StreamOptions{
		VideoTrack: 1,
		AudioTrack: 2,
		Bitrate:    "3M",
		Subtitle: options.SubtitleCastOptions{
			Path:         "embedded:3",
			FontSize:     40,
			Italic:       true,
			DelaySeconds: -1.5,
			BurnIn:       true,
		},
	}

	rec := httptest.NewRecorder()
	NewHTTPServer(a).handleState(rec, httptest.NewRequest(http.MethodGet, "/state", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	sel, ok := raw["selection"].(map[string]any)
	if !ok {
		t.Fatalf("no selection in %s", rec.Body)
	}
	want := map[string]any{
		"videoTrack":   1.0,
		"audioTrack":   2.0,
		"quality":      "3M",
		"subtitlePath": "embedded:3",
		"fontSize":     40.0,
		"bold":         false,
		"italic":       true,
		"delaySeconds": -1.5,
		"burnIn":       true,
	}
	for k, v := range want {
		if sel[k] != v {
			t.Errorf("selection.%s = %v, want %v", k, sel[k], v)
		}
	}
}

func TestStateOmitsSelectionBeforeFirstCast(t *testing.T) {
	a, _ := newTestApp(t)
	rec := httptest.NewRecorder()
	NewHTTPServer(a).handleState(rec, httptest.NewRequest(http.MethodGet, "/state", nil))
	var raw map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	if _, ok := raw["selection"]; ok {
		t.Fatalf("selection present before any cast: %s", rec.Body)
	}
}

// /track-info for a web URL that is playing must come from the casting
// instance's cache, never a fresh extraction (which would open a browser).
func TestTrackInfoForWebURLUsesCachedExtraction(t *testing.T) {
	a, _ := newTestApp(t)
	a.RemoteManager = remote.NewManager(true)
	page := "https://example.test/film/1/"
	dir := folders.Video(page)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "#EXTM3U\n" +
		"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"aud\",LANGUAGE=\"en\",NAME=\"English\",URI=\"a/en.m3u8\"\n" +
		"#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID=\"aud\",LANGUAGE=\"tr\",NAME=\"Turkish\",URI=\"a/tr.m3u8\"\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080,AUDIO=\"aud\"\n" +
		"v/1080.m3u8\n"
	if err := os.WriteFile(filepath.Join(dir, "playlist.m3u8"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	extraction := `{"URL":"` + page + `","Title":"Film","ManifestURL":"https://cdn.example.test/master.m3u8"}`
	if err := os.WriteFile(filepath.Join(dir, "extraction.json"), []byte(extraction), 0o644); err != nil {
		t.Fatal(err)
	}

	first, err := a.GetTrackDisplayInfo(page)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.AudioTracks) != 2 || len(first.VideoTracks) != 1 {
		t.Fatalf("tracks = %+v", first)
	}

	// Once loaded, the media stays in memory: removing the on-disk cache
	// must not matter, so nothing is re-read or re-extracted.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	second, err := a.GetTrackDisplayInfo(page)
	if err != nil {
		t.Fatalf("second lookup went back to disk or extraction: %v", err)
	}
	if len(second.AudioTracks) != 2 {
		t.Fatalf("second lookup tracks = %+v", second)
	}
}
