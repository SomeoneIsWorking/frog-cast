package main

import (
	"fmt"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func fmtSec(s float64) string {
	if s < 0 {
		s = 0
	}
	total := int(s)
	h := total / 3600
	m := (total % 3600) / 60
	sec := total % 60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func fmtSpeed(bps int64) string {
	if bps <= 0 {
		return "0 B/s"
	}
	f := float64(bps)
	switch {
	case f >= 1<<20:
		return fmt.Sprintf("%.1f MB/s", f/(1<<20))
	case f >= 1<<10:
		return fmt.Sprintf("%.1f KB/s", f/(1<<10))
	default:
		return fmt.Sprintf("%d B/s", bps)
	}
}

func fmtEta(sec int64) string {
	if sec <= 0 || sec >= 8640000 {
		return "—"
	}
	m := sec / 60
	s := sec % 60
	return fmt.Sprintf("%dm %ds", m, s)
}
