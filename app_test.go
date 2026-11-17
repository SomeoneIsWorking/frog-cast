package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vishen/go-chromecast/application"
	cast_proto "github.com/vishen/go-chromecast/cast/proto"

	"wails-cast/pkg/castapi"
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
	// The size changed, so the receiver is told at once, then the VTT is re-sent.
	if !slices.Equal(fake.custom, []string{"subtitleSize", "subtitles"}) {
		t.Fatalf("receiver messages = %v", fake.custom)
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

func intp(v int) *int       { return &v }
func strp(v string) *string { return &v }

// playingApp is a test app casting /media/film.mkv to a device, reported at
// position seconds as of ago.
func playingApp(t *testing.T, status string, position float64, ago time.Duration, opts options.StreamOptions) (*App, *fakeReceiver) {
	t.Helper()
	a, fake := newTestApp(t)
	a.playbackState = PlaybackState{
		Status:      status,
		MediaPath:   "/media/film.mkv",
		MediaName:   "film.mkv",
		DeviceURL:   "192.168.1.50",
		CurrentTime: position,
		Duration:    7200,
	}
	a.positionAt = time.Now().Add(-ago)
	a.streamOptions = &opts
	return a, fake
}

func TestChangeTracksRecastsAtCurrentPosition(t *testing.T) {
	a, fake := playingApp(t, "PLAYING", 100, 30*time.Second, options.StreamOptions{
		Subtitle: options.SubtitleCastOptions{Path: "none"},
	})

	state, err := a.ChangeTracks(castapi.TrackChange{AudioTrack: intp(1)})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.loads) != 1 {
		t.Fatalf("loads = %d, want 1", len(fake.loads))
	}
	load := fake.loads[0]
	if load.StartTime < 129 || load.StartTime > 131 {
		t.Fatalf("StartTime = %d, want about 130", load.StartTime)
	}
	if load.Paused {
		t.Fatal("a playing cast was reloaded paused")
	}
	if state.Selection == nil || state.Selection.AudioTrack != 1 {
		t.Fatalf("selection = %+v", state.Selection)
	}
	if state.Status != "LOADING" {
		t.Fatalf("status = %s, want LOADING until the device reports", state.Status)
	}
	if got := a.historyStore.GetAll()[0].CastOptions.AudioTrack; got != 1 {
		t.Fatalf("history audio track = %d", got)
	}
}

func TestChangeTracksKeepsPausedCastPaused(t *testing.T) {
	a, fake := playingApp(t, "PAUSED", 200, time.Minute, options.StreamOptions{
		Bitrate:  "5M",
		Subtitle: options.SubtitleCastOptions{Path: "none"},
	})

	if _, err := a.ChangeTracks(castapi.TrackChange{Quality: strp("2M")}); err != nil {
		t.Fatal(err)
	}
	if len(fake.loads) != 1 {
		t.Fatalf("loads = %d", len(fake.loads))
	}
	if fake.loads[0].StartTime != 200 || !fake.loads[0].Paused {
		t.Fatalf("load = %+v, want paused at 200", fake.loads[0])
	}
}

func TestChangeTracksExternalSubtitleIsLive(t *testing.T) {
	a, fake := playingApp(t, "PLAYING", 50, 0, options.StreamOptions{
		Subtitle: options.SubtitleCastOptions{Path: "none"},
	})

	state, err := a.ChangeTracks(castapi.TrackChange{SubtitlePath: strp("external:/media/film.srt")})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake.loads) != 0 {
		t.Fatal("an external subtitle change re-cast the media")
	}
	if len(fake.custom) != 1 {
		t.Fatalf("subtitles not sent: %v", fake.custom)
	}
	if state.Selection.SubtitlePath != "external:/media/film.srt" {
		t.Fatalf("selection = %+v", state.Selection)
	}
}

func TestChangeTracksBurnInSubtitleRecasts(t *testing.T) {
	a, fake := playingApp(t, "PLAYING", 50, 0, options.StreamOptions{
		Subtitle: options.SubtitleCastOptions{Path: "none", BurnIn: true},
	})

	if _, err := a.ChangeTracks(castapi.TrackChange{SubtitlePath: strp("embedded:2")}); err != nil {
		t.Fatal(err)
	}
	if len(fake.loads) != 1 {
		t.Fatalf("loads = %d, want a re-cast for burn-in", len(fake.loads))
	}
}

func TestChangeTracksNeedsACast(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.ChangeTracks(castapi.TrackChange{AudioTrack: intp(1)}); err == nil {
		t.Fatal("expected an error with nothing playing")
	}
}

// A replaced connection's IDLE report must not stop the cast that replaced
// it, and the old media's end during a load must not either.
func TestDeviceReportsDuringReload(t *testing.T) {
	a, fake := playingApp(t, "LOADING", 130, 0, options.StreamOptions{})
	idle := `{"type":"MEDIA_STATUS","status":[{"currentTime":12,"playerState":"IDLE","idleReason":"INTERRUPTED"}]}`

	a.handleChromecastMessage(&fakeReceiver{}, castMessage(idle))
	a.handleChromecastMessage(fake, castMessage(idle))
	if a.playbackState.Status != "LOADING" || a.playbackState.CurrentTime != 130 {
		t.Fatalf("state = %+v after old media's IDLE", a.playbackState)
	}

	playing := `{"type":"MEDIA_STATUS","status":[{"currentTime":131,"playerState":"PLAYING"}]}`
	a.handleChromecastMessage(fake, castMessage(playing))
	if a.playbackState.Status != "PLAYING" || a.playbackState.CurrentTime != 131 {
		t.Fatalf("state = %+v after the new media plays", a.playbackState)
	}
}

func castMessage(payload string) *cast_proto.CastMessage {
	return &cast_proto.CastMessage{PayloadUtf8: &payload}
}

// The casting instance keeps each media's subtitle offset and applies it on
// every later cast, whichever controller starts it.
func TestSubtitleOffsetIsKeptPerMediaByTheCastingInstance(t *testing.T) {
	a, fake := newTestApp(t)
	media := "/media/offset-film.mkv"
	if _, err := a.CastToDevice("192.168.1.50", media, &options.CastOptions{SubtitlePath: "none"}); err != nil {
		t.Fatal(err)
	}
	if err := a.UpdateSubtitleSettings(options.SubtitleCastOptions{DelaySeconds: 2.5}); err != nil {
		t.Fatal(err)
	}
	if got := a.historyStore.SubtitleDelay(media); got != 2.5 {
		t.Fatalf("kept offset = %v, want 2.5", got)
	}

	// A remote controller starts the same media again with its own style.
	srv := NewHTTPServer(a)
	body := `{"id":"` + media + `","deviceIp":"192.168.1.50","subtitle":{"fontSize":60,"bold":true}}`
	rec := httptest.NewRecorder()
	srv.handlePlay(rec, httptest.NewRequest(http.MethodPost, "/play", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("play: %d %s", rec.Code, rec.Body)
	}
	if len(fake.loads) != 2 {
		t.Fatalf("loads = %d", len(fake.loads))
	}
	sel := a.currentPlaybackState().Selection
	if sel.DelaySeconds != 2.5 {
		t.Fatalf("offset on recast = %v, want 2.5", sel.DelaySeconds)
	}
	if sel.FontSize != 60 || !sel.Bold {
		t.Fatalf("controller style not used: %+v", sel)
	}
	if got := a.historyStore.SubtitleDelay(media); got != 2.5 {
		t.Fatalf("re-adding to history dropped the offset: %v", got)
	}
}
