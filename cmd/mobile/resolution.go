package main

import (
	"strconv"
	"strings"

	"frog-cast/pkg/castapi"
	"frog-cast/pkg/hls"
)

const prefMaxHeight = "maxHeight"

// The tallest video a web page may cast at; some TVs refuse sizes above 1080p.
var (
	resolutionLabels  = []string{"Highest", "Up to 1080p", "Up to 720p", "Up to 480p"}
	resolutionHeights = []int{0, 1080, 720, 480}
)

// variantFor is the video variant to cast: the tallest no taller than
// maxHeight (0 = any), else the shortest. Variants of unknown size lose to
// any known one.
func variantFor(tracks []castapi.VideoTrackDisplayItem, maxHeight int) int {
	best, bestHeight := 0, -1
	shortest, shortestHeight := 0, -1
	for _, t := range tracks {
		h := variantHeight(t.Resolution)
		if h <= 0 {
			continue
		}
		if (maxHeight == 0 || h <= maxHeight) && h > bestHeight {
			best, bestHeight = t.Index, h
		}
		if shortestHeight < 0 || h < shortestHeight {
			shortest, shortestHeight = t.Index, h
		}
	}
	switch {
	case bestHeight > 0:
		return best
	case shortestHeight > 0:
		return shortest
	default:
		return 0
	}
}

// variantHeight is the height of an HLS RESOLUTION ("1920x1080"); 0 if unknown.
func variantHeight(resolution string) int {
	_, h, ok := strings.Cut(resolution, "x")
	if !ok {
		return 0
	}
	n, err := strconv.Atoi(h)
	if err != nil {
		return 0
	}
	return n
}

// manifestVariants lists a master playlist's variants as the instance would.
func manifestVariants(manifest string) []castapi.VideoTrackDisplayItem {
	m, err := hls.ParseManifest(manifest)
	if err != nil {
		return nil
	}
	var out []castapi.VideoTrackDisplayItem
	for _, t := range m.VideoTracks {
		out = append(out, castapi.VideoTrackDisplayItem{Index: t.Index, Resolution: t.Resolution, Codecs: t.Codecs})
	}
	return out
}
