package stream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"frog-cast/pkg/filehelper"
	"frog-cast/pkg/mix"
)

func TestProcessSubtitlesLeavesTheLinkedSourceAlone(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "Turkish.vtt")
	original := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.000\nMerhaba\n"
	if err := os.WriteFile(source, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "cache", "subtitles.vtt")
	if err := filehelper.EnsureSymlink(source, target); err != nil {
		t.Fatal(err)
	}

	for range 2 {
		if _, err := ProcessSubtitles(mix.File(target), mix.FileTarget(target), true, 2, true, false); err != nil {
			t.Fatal(err)
		}
		if err := filehelper.EnsureSymlink(source, target); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ProcessSubtitles(mix.File(target), mix.FileTarget(target), true, 2, true, false); err != nil {
		t.Fatal(err)
	}

	if got, _ := os.ReadFile(source); string(got) != original {
		t.Fatalf("source rewritten:\n%s", got)
	}
	got, _ := os.ReadFile(target)
	if !strings.Contains(string(got), "00:00:03.000 --> 00:00:04.000") || strings.Count(string(got), "<b>") != 1 {
		t.Fatalf("output:\n%s", got)
	}
}
