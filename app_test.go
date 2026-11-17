package main

import (
	"os"
	"sync"
	"testing"
	"time"

	"github.com/vishen/go-chromecast/application"

	"wails-cast/pkg/options"
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
	a.subtitlePath = "external:/media/show.srt"

	if err := a.UpdateSubtitleSettings(options.SubtitleCastOptions{FontSize: 30}); err != nil {
		t.Fatal(err)
	}
	if a.subtitlePath != "external:/media/show.srt" {
		t.Fatalf("subtitle source changed to %q", a.subtitlePath)
	}
	if len(fake.custom) != 1 || fake.custom[0] != "subtitles" {
		t.Fatalf("external subtitles not re-sent: %v", fake.custom)
	}
}

func TestUpdateSubtitleSettingsSeeksToExtrapolatedPosition(t *testing.T) {
	a, fake := newTestApp(t)
	a.subtitlePath = "none"
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
