package main

import "fmt"

// fmtSec is a playback position, m:ss or h:mm:ss.
func fmtSec(s float64) string {
	total := int(max(s, 0))
	h, m, sec := total/3600, total%3600/60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, sec)
	}
	return fmt.Sprintf("%d:%02d", m, sec)
}

// fmtBytes is a size in B, KB, MB or GB.
func fmtBytes(n int64) string {
	f := float64(n)
	switch {
	case n <= 0:
		return "0 B"
	case f >= 1<<30:
		return fmt.Sprintf("%.1f GB", f/(1<<30))
	case f >= 1<<20:
		return fmt.Sprintf("%.1f MB", f/(1<<20))
	case f >= 1<<10:
		return fmt.Sprintf("%.1f KB", f/(1<<10))
	default:
		return fmt.Sprintf("%d B", n)
	}
}

func fmtSpeed(bps int64) string {
	return fmtBytes(bps) + "/s"
}

// fmtEta is a download's time left; qBittorrent reports 8640000 for unknown.
func fmtEta(sec int64) string {
	if sec <= 0 || sec >= 8640000 {
		return "—"
	}
	if sec >= 3600 {
		return fmt.Sprintf("%dh %dm left", sec/3600, sec%3600/60)
	}
	return fmt.Sprintf("%dm %ds left", sec/60, sec%60)
}
