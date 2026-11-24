package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"frog-cast/pkg/castapi"
)

// fakeInstance answers /control and records what it was sent.
func fakeInstance(t *testing.T) (*httptest.Server, func() []map[string]any) {
	t.Helper()
	var mu sync.Mutex
	var sent []map[string]any
	var inFlight atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/control" {
			if inFlight.Add(1) > 1 {
				t.Error("two control requests at once")
			}
			defer inFlight.Add(-1)
			time.Sleep(20 * time.Millisecond)
			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			sent = append(sent, body)
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]any{"ok": true, "state": castapi.PlaybackState{Status: "PLAYING", Volume: 0.05}})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []map[string]any {
		mu.Lock()
		defer mu.Unlock()
		return append([]map[string]any(nil), sent...)
	}
}

func waitSent(t *testing.T, sent func() []map[string]any, n int) []map[string]any {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for len(sent()) < n && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	got := sent()
	if len(got) != n {
		t.Fatalf("sent %v, want %d commands", got, n)
	}
	return got
}

// The TV's volume runs 0–100 and is used around 7: each button is one notch,
// and quick taps add up even before the TV reports back.
func TestPlayingVolumeStepsAndRecast(t *testing.T) {
	srv, sent := fakeInstance(t)
	a := test.NewTempApp(t)
	w := test.NewWindow(nil)
	w.Resize(fyne.NewSize(390, 844))
	u := &ui{fyneApp: a, window: w}
	u.openSession("fake", castapi.New(srv.URL, ""))
	p := u.session.playing
	p.show(&castapi.PlaybackState{Status: "PLAYING", MediaName: "Film", DeviceURL: "tv", Volume: 0.07})
	p.stepVolume(1)
	p.stepVolume(1)
	p.stepVolume(-1)
	// The first tap goes out at once; the two made during it collapse into one.
	got := waitSent(t, sent, 2)
	want := []float64{0.08, 0.08}
	for i, cmd := range got {
		if cmd["action"] != "volume" || !near(cmd["value"].(float64), want[i]) {
			t.Fatalf("command %d = %v, want volume %v", i, cmd, want[i])
		}
	}
	// A poll with the TV's older volume doesn't undo the taps.
	p.show(&castapi.PlaybackState{Status: "PLAYING", MediaName: "Film", DeviceURL: "tv", Volume: 0.07})
	if p.level.Text != "8" {
		t.Fatalf("level after a stale poll = %q", p.level.Text)
	}
	p.recast()
	if got := waitSent(t, sent, 3); got[2]["action"] != "recast" {
		t.Fatalf("sent %v", got)
	}
	u.closeSession()
}

func near(a, b float64) bool {
	return a-b < 1e-9 && b-a < 1e-9
}
