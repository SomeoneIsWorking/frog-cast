package main

import (
	"testing"
	"time"

	"frog-cast/pkg/castapi"
)

func TestHistoryDetail(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	cases := map[castapi.HistoryItem]string{
		{Path: "https://www.example.test/film/x/", Timestamp: "2026-11-18T18:00:00Z"}: "2 h ago · example.test",
		{Path: "/mnt/TV/Frieren/S01E01.mkv", Timestamp: "2026-11-18T19:55:00Z"}:       "5 min ago · Frieren",
		{Path: "/mnt/TV/Frieren/S01E02.mkv", Timestamp: "2026-11-15T19:00:00Z"}:       "3 days ago · Frieren",
		{Path: "https://example.test/a", Timestamp: "not a time"}:                     "example.test",
	}
	for it, want := range cases {
		if got := historyDetail(it, now); got != want {
			t.Errorf("historyDetail(%+v) = %q, want %q", it, got, want)
		}
	}
}
