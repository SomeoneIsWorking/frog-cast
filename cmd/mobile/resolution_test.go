package main

import (
	"testing"

	"frog-cast/pkg/castapi"
)

func TestVariantFor(t *testing.T) {
	tracks := []castapi.VideoTrackDisplayItem{
		{Index: 0, Resolution: "3200x1800"},
		{Index: 1, Resolution: "1920x1080"},
		{Index: 2, Resolution: "1280x720"},
		{Index: 3, Resolution: ""},
	}
	cases := map[int]int{0: 0, 1080: 1, 720: 2, 480: 2}
	for maxHeight, want := range cases {
		if got := variantFor(tracks, maxHeight); got != want {
			t.Errorf("variantFor(max %d) = %d, want %d", maxHeight, got, want)
		}
	}
	if got := variantFor([]castapi.VideoTrackDisplayItem{{Index: 0}}, 1080); got != 0 {
		t.Errorf("unknown sizes: got %d, want 0", got)
	}
}

func TestVideoLabel(t *testing.T) {
	cases := map[castapi.VideoTrackDisplayItem]string{
		{Index: 0, Resolution: "1920x1036", Codecs: "av01.0.08M.08,mp4a.40.2"}: "1036p · AV1",
		{Index: 1, Resolution: "1280x720", Codecs: "mp4a.40.2,avc1.64001f"}:    "720p · H.264",
		{Index: 2}: "Track 2",
	}
	for track, want := range cases {
		if got := videoLabel(track); got != want {
			t.Errorf("videoLabel(%+v) = %q, want %q", track, got, want)
		}
	}
}
