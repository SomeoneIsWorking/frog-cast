// Command extract runs the app's extractor against a page URL and prints what
// the page reported and what was found, for testing Lua site scripts without
// the GUI.
//
//	go run ./cmd/extract [-json out.json] <page-url>
//	go run ./cmd/extract -segments <out-dir> [-video N] [-audio N] <page-url>
//
// With -segments it also runs the same download + transcode path a cast uses
// (RemoteManager -> RemoteHandler) and writes the served playlists and the
// first video/audio segment into <out-dir>, so they can be checked with ffprobe.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"wails-cast/pkg/extractor"
	"wails-cast/pkg/options"
	"wails-cast/pkg/remote"
	"wails-cast/pkg/stream"
	"wails-cast/pkg/webextract"
)

// The WebKit host's main loop runs on the main thread.
func init() {
	runtime.LockOSThread()
}

func main() {
	segmentsDir := flag.String("segments", "", "also download+transcode the first segments into this directory")
	videoTrack := flag.Int("video", 0, "video track index for -segments")
	audioTrack := flag.Int("audio", 0, "audio track index for -segments")
	out := flag.String("json", "", "write the result to this file")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/extract [-json out] [-segments dir [-video N] [-audio N]] <page-url>")
		os.Exit(2)
	}
	pageURL := flag.Arg(0)

	var err error
	webextract.RunMainLoop(func() {
		if *segmentsDir != "" {
			err = runSegments(pageURL, *segmentsDir, *videoTrack, *audioTrack)
		} else {
			err = extract(pageURL, *out)
		}
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func extract(pageURL, out string) error {
	start := time.Now()
	trace := func(m webextract.Message) {
		fmt.Printf("%5.1fs %-11s top=%-5v %s  [frame %s]\n", time.Since(start).Seconds(), m.Type, m.Top, cut(m.URL, 110), cut(m.Frame, 80))
	}
	res, err := extractor.Extract(pageURL, trace)
	if err != nil {
		return err
	}
	fmt.Printf("found in %s\ntitle: %s\nmanifest: %s (%d bytes, master=%v)\nheaders: %v\ncookies: %d\n",
		time.Since(start).Round(100*time.Millisecond), res.Title, res.ManifestURL, len(res.Manifest),
		strings.Contains(res.Manifest, "#EXT-X-STREAM-INF"), res.Headers, len(res.Cookies))
	fmt.Printf("subtitles: %d\n", len(res.Subtitles))
	for _, s := range res.Subtitles {
		fmt.Printf("  - %s %s\n", s.Label, s.URL)
	}
	if out == "" {
		return nil
	}
	data, err := json.MarshalIndent(res, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(out, data, 0o644)
}

func cut(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func runSegments(pageURL, dir string, videoTrack, audioTrack int) error {
	ctx := context.Background()
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	media, err := remote.NewManager(false).GetMedia(pageURL)
	if err != nil {
		return fmt.Errorf("get media: %w", err)
	}
	fmt.Printf("Title: %s\nVideo tracks: %d, audio tracks: %d\n", media.Title, len(media.Manifest.VideoTracks), len(media.Manifest.AudioTracks))

	handler, err := stream.NewRemoteHandler(ctx, media, options.StreamOptions{
		Subtitle:         options.SubtitleCastOptions{Path: "none"},
		VideoTrack:       videoTrack,
		AudioTrack:       audioTrack,
		NoTranscodeCache: true,
	}, "")
	if err != nil {
		return fmt.Errorf("remote handler: %w", err)
	}

	master, err := handler.ServeManifestPlaylist(ctx)
	if err != nil {
		return fmt.Errorf("master playlist: %w", err)
	}
	os.WriteFile(filepath.Join(dir, "playlist.m3u8"), []byte(master), 0644)

	tracks := []string{"video"}
	if handler.AudioManager != nil {
		tracks = append(tracks, "audio")
	}
	for _, t := range tracks {
		pl, err := handler.ServeTrackPlaylist(ctx, t)
		if err != nil {
			return fmt.Errorf("%s playlist: %w", t, err)
		}
		os.WriteFile(filepath.Join(dir, t+".m3u8"), []byte(pl), 0644)
		seg, err := handler.ServeSegment(ctx, t, 0)
		if err != nil {
			return fmt.Errorf("%s segment 0: %w", t, err)
		}
		out := filepath.Join(dir, t+"_segment_0.ts")
		if err := os.WriteFile(out, seg.Buffer, 0644); err != nil {
			return err
		}
		fmt.Printf("%s segment 0: %d bytes -> %s\n", t, len(seg.Buffer), out)
	}
	return nil
}
