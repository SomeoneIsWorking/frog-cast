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

func TestManifestVariants(t *testing.T) {
	master := "#EXTM3U\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=9000000,RESOLUTION=3200x1800\nhigh.m3u8\n" +
		"#EXT-X-STREAM-INF:BANDWIDTH=5000000,RESOLUTION=1920x1080\nmid.m3u8\n"
	if got := variantFor(manifestVariants(master), 1080); got != 1 {
		t.Errorf("1080p cap picks %d, want 1", got)
	}
}
