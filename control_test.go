package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"frog-cast/pkg/castapi"
	"frog-cast/pkg/options"
	"frog-cast/pkg/remote"
)

// A TV that slept through a long pause closes the session; recast loads the
// same media on it again where it was, playing.
func TestRecastAfterTheTVClosedTheSession(t *testing.T) {
	a, fake := playingApp(t, "PAUSED", 3000, time.Minute, options.StreamOptions{
		AudioTrack: 1,
		Subtitle:   options.SubtitleCastOptions{Path: "none"},
	})
	a.handleChromecastMessage(a.App, castMessage(`{"type":"CLOSE"}`))
	if st := a.currentPlaybackState(); st.Status != "STOPPED" {
		t.Fatalf("status after CLOSE = %s", st.Status)
	}

	rec := postJSON(t, NewHTTPServer(a).handleControl, "/control", map[string]any{"action": "recast"})
	if rec.Code != http.StatusOK {
		t.Fatalf("recast = %d %s", rec.Code, rec.Body)
	}
	if len(fake.loads) != 1 {
		t.Fatalf("loads = %d", len(fake.loads))
	}
	if load := fake.loads[0]; load.StartTime != 3000 || load.Paused {
		t.Fatalf("load = start %d paused %v, want 3000 playing", load.StartTime, load.Paused)
	}
	if st := a.currentPlaybackState(); st.DeviceURL != "192.168.1.50" || st.Selection == nil || st.Selection.AudioTrack != 1 {
		t.Fatalf("state = %+v", st)
	}
}

func TestRecastNeedsAPreviousCast(t *testing.T) {
	a, _ := newTestApp(t)
	if _, err := a.Recast(); err == nil {
		t.Fatal("recast with nothing cast")
	}
}

// Reading a download's progress must not touch the site: a failed probe used
// to drop the page's whole cache mid-download.
func TestDownloadStatusKeepsTheCacheWhenTheSiteFails(t *testing.T) {
	a, _ := newTestApp(t)
	a.RemoteManager = remote.NewManager(true)
	h := NewHTTPServer(a)
	page := "https://example.test/watch/45/"
	cdn, expired := expiringCDN(t)
	seedPage(t, a, page, cdn.URL+"/tok/master.m3u8")
	*expired = true

	get := httptest.NewRecorder()
	h.handleDownload(get, httptest.NewRequest(http.MethodGet, "/download?type=video&track=0&url="+url.QueryEscape(page), nil))
	var status castapi.DownloadStatus
	if get.Code != http.StatusOK || json.Unmarshal(get.Body.Bytes(), &status) != nil || status.Total != 1 {
		t.Fatalf("status = %d %s", get.Code, get.Body)
	}
	if !a.RemoteManager.HasExtraction(page) {
		t.Fatal("a status read dropped the page's cache")
	}
}
