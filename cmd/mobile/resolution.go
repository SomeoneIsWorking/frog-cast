package main

import (
	"fmt"
	"strconv"
	"strings"

	"frog-cast/pkg/castapi"
)

// defaultMaxHeight is the tallest variant chosen by default: variants are
// re-encoded at their own size, and many TVs refuse more than 1080p.
const defaultMaxHeight = 1080

// variantFor is the video variant offered first: the tallest no taller than
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

// videoLabel names a variant by height and codec, e.g. "1036p · AV1".
func videoLabel(t castapi.VideoTrackDisplayItem) string {
	size := fmt.Sprintf("Track %d", t.Index)
	if h := variantHeight(t.Resolution); h > 0 {
		size = fmt.Sprintf("%dp", h)
	}
	if codec := videoCodec(t.Codecs); codec != "" {
		return size + " · " + codec
	}
	return size
}

// videoCodec is the video codec of an HLS CODECS list, by name.
func videoCodec(codecs string) string {
	names := map[string]string{"avc1": "H.264", "avc3": "H.264", "hvc1": "HEVC", "hev1": "HEVC", "av01": "AV1", "vp09": "VP9"}
	for _, c := range strings.Split(codecs, ",") {
		prefix, _, _ := strings.Cut(strings.TrimSpace(c), ".")
		if name, ok := names[prefix]; ok {
			return name
		}
	}
	return ""
}
