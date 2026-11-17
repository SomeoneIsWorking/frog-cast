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
	"fyne.io/fyne/v2/theme"

	"wails-cast/pkg/castapi"
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
	a.Settings().SetTheme(theme.DarkTheme())
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
	s.tabs.SelectIndex(0)
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
	if st := s.playing.state; st != nil && st.Status != "STOPPED" {
		s.showTracksSheet(st)
		snap("6-tracks")
		closeOverlays(w)
		s.showSubtitleSheet(st)
		snap("7-subtitles")
		closeOverlays(w)
	}

	s.tabs.SelectIndex(2)
	snap("8-torrents")
	s.tabs.SelectIndex(3)
	snap("9-manage")
	u.closeSession()
}

func closeOverlays(w fyne.Window) {
	for w.Canvas().Overlays().Top() != nil {
		w.Canvas().Overlays().Remove(w.Canvas().Overlays().Top())
	}
}
