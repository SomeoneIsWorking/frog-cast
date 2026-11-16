// Command extract runs the app's real extractor against a page URL and prints
// what it found, for testing Lua extraction scripts without the GUI.
//
//	go run ./cmd/extract <page-url>
//	go run ./cmd/extract -segments <out-dir> [-video N] [-audio N] <page-url>
//
// With -segments it also runs the same download + transcode path a cast uses
// (RemoteManager -> RemoteHandler) and writes the served playlists and the
// first video/audio segment into <out-dir>, so they can be checked with ffprobe.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wails-cast/pkg/extractor"
	"wails-cast/pkg/options"
	"wails-cast/pkg/remote"
	"wails-cast/pkg/stream"
)

func main() {
	segmentsDir := flag.String("segments", "", "also download+transcode the first segments into this directory")
	videoTrack := flag.Int("video", 0, "video track index for -segments")
	audioTrack := flag.Int("audio", 0, "audio track index for -segments")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: go run ./cmd/extract [-segments dir [-video N] [-audio N]] <page-url>")
		os.Exit(2)
	}
	pageURL := flag.Arg(0)

	if *segmentsDir != "" {
		if err := runSegments(pageURL, *segmentsDir, *videoTrack, *audioTrack); err != nil {
			fmt.Fprintf(os.Stderr, "stream test failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	result, err := extractor.Extract(pageURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "extraction failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Title:    %s\n", result.Title)
	fmt.Printf("URL:      %s\n", result.URL)
	fmt.Printf("Headers:  %v\n", result.Headers)
	fmt.Printf("Cookies:  %d\n", len(result.Cookies))
	fmt.Printf("Subtitles: %d\n", len(result.Subtitles))
	for _, s := range result.Subtitles {
		fmt.Printf("  - %s %s\n", s.Label, s.URL)
	}
	lines := strings.Split(result.Manifest, "\n")
	if len(lines) > 15 {
		lines = lines[:15]
	}
	fmt.Printf("Manifest (%d bytes):\n%s\n", len(result.Manifest), strings.Join(lines, "\n"))
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
