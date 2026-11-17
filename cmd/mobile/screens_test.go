package main

import (
	"flag"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"frog-cast/pkg/castapi"
)

var (
	shots = flag.String("shots", "", "directory to write screen captures to")
	base  = flag.String("base", "", "instance to render against, e.g. http://127.0.0.1:9999 (read-only calls only)")
)

// TestRenderScreens captures each screen against a live instance, for
// looking at the layout without a phone:
//
//	go test ./cmd/mobile -run RenderScreens -args -shots DIR -base URL
func TestRenderScreens(t *testing.T) {
	if *shots == "" || *base == "" {
		t.Skip("pass -shots and -base to render")
	}
	a := test.NewTempApp(t)
	a.Settings().SetTheme(frogTheme{})
	w := test.NewWindow(nil)
	w.Resize(fyne.NewSize(390, 844))
	u := &ui{fyneApp: a, window: w}

	snap := func(name string) {
		time.Sleep(4 * time.Second)
		f, err := os.Create(filepath.Join(*shots, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, w.Canvas().Capture()); err != nil {
			t.Fatal(err)
		}
	}

	u.showConnect()
	snap("1-connect")

	u.openSession("fedora", castapi.New(*base, ""))
	s := u.session
	time.Sleep(2 * time.Second)
	s.tabs.Select(tabLibrary)
	snap("2-library")

	tree, err := s.client.LibraryTree()
	if err != nil || len(tree.Shows) == 0 {
		t.Fatalf("library: %v", err)
	}
	show := tree.Shows[0]
	s.library.openShow(show)
	snap("3-episodes")
	ep := show.Seasons[0].Episodes[0]
	s.showPlaySheet(ep.Path, show.Name+" · "+episodeTitle(ep))
	snap("4-play-sheet")
	closeOverlays(w)

	s.showPlaying()
	snap("5-playing")
	s.playing.setPolling(false)
	s.playing.show(&castapi.PlaybackState{Status: "PLAYING", MediaName: "Frieren - Beyond Journey's End · E01",
		DeviceURL: "local", CurrentTime: 312, Duration: 1440, Volume: 0.6})
	snap("5b-playing-active")
	if st := s.playing.state; st != nil && st.Status != "STOPPED" {
		s.showTracksSheet(st)
		snap("6-tracks")
		closeOverlays(w)
		s.showSubtitleSheet(st)
		snap("7-subtitles")
		closeOverlays(w)
	}

	s.tabs.Select(tabDownloads)
	snap("8-downloads")
	s.tabs.Select(tabManage)
	snap("9-manage")
	s.tabs.Select(tabLibrary)
	s.library.pages.back()
	s.library.pages.back()
	s.library.pages.push("History", s.history.content)
	s.history.show([]castapi.HistoryItem{
		{Path: "https://hlsjs.video-dev.org/demo/", Name: "hls.js demo", Timestamp: time.Now().Add(-2 * time.Hour).Format(time.RFC3339)},
		{Path: "/mnt/TV/Frieren/Season 1/S01E03.mkv", Name: "Frieren S01E03", Timestamp: time.Now().Add(-26 * time.Hour).Format(time.RFC3339)},
	})
	snap("12-history")
	s.library.pages.back()
	s.library.pages.push("Web page", s.web.content)
	s.web.link.SetText("https://www.example.test/film/sevimli-canavarlar-monsters-inc-2/")
	snap("10-web")
	// A page the instance has extracted before; reading its tracks is read-only.
	s.web.openPage("https://www.fullhdfilmizlesene.now/film/sevimli-canavarlar-monsters-inc-2/")
	snap("11-cast-options")
	u.closeSession()
}

func closeOverlays(w fyne.Window) {
	for w.Canvas().Overlays().Top() != nil {
		w.Canvas().Overlays().Remove(w.Canvas().Overlays().Top())
	}
}
