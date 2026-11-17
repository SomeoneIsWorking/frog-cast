// Command webextract finds a web page's HLS stream with pkg/webextract, for
// trying site rules without the app.
//
//	go run ./cmd/webextract -url https://example.test/film/ -click '#play'
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"

	"wails-cast/pkg/webextract"
)

func init() {
	runtime.LockOSThread()
}

type list []string

func (l *list) String() string     { return strings.Join(*l, ",") }
func (l *list) Set(v string) error { *l = append(*l, v); return nil }

func main() {
	var rules webextract.Rules
	page := flag.String("url", "", "page to extract")
	visible := flag.Bool("show", false, "show the window, to click play by hand")
	timeout := flag.Duration("timeout", 60*time.Second, "give up after")
	out := flag.String("json", "", "write the result to this file")
	var click, block, body, inline list
	flag.Var(&click, "click", "selector to click as it appears (repeatable)")
	flag.Var(&block, "block", "URL substring to block (repeatable)")
	flag.Var(&body, "body", "url-substring=regex finding the stream URL in a response (repeatable)")
	flag.Var(&inline, "inline", "frame-url-substring=regex finding it in a frame's inline scripts (repeatable)")
	flag.Parse()
	rules.Click, rules.Block = click, block
	rules.BodyRules, rules.InlineRules = textRules(body), textRules(inline)

	failed := false
	webextract.RunMainLoop(func() {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		defer cancel()
		start := time.Now()
		trace := func(m webextract.Message) {
			fmt.Printf("%5.1fs %-11s top=%-5v %s  [frame %s]\n", time.Since(start).Seconds(), m.Type, m.Top, cut(m.URL, 110), cut(m.Frame, 80))
			if m.Cookies != "" && m.Type != "frame" {
				fmt.Printf("        cookies: %s\n", cut(m.Cookies, 200))
			}
		}
		res, err := webextract.Extract(ctx, *page, rules, webextract.Options{Visible: *visible, Trace: trace})
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			failed = true
			return
		}
		fmt.Printf("found in %s\ntitle: %s\nmanifest: %s (%d bytes, master=%v)\nheaders: %v\ncookies: %d\nsubtitles: %d\n",
			time.Since(start).Round(100*time.Millisecond), res.Title, res.ManifestURL, len(res.Manifest),
			strings.Contains(res.Manifest, "#EXT-X-STREAM-INF"), res.Headers, len(res.Cookies), len(res.Subtitles))
		if *out != "" {
			data, _ := json.MarshalIndent(res, "", "  ")
			if err := os.WriteFile(*out, data, 0o644); err != nil {
				fmt.Fprintln(os.Stderr, "error:", err)
				failed = true
			}
		}
	})
	if failed {
		os.Exit(1)
	}
}

func cut(s string, n int) string {
	if len(s) > n {
		return s[:n] + "…"
	}
	return s
}

func textRules(l list) []webextract.TextRule {
	var out []webextract.TextRule
	for _, v := range l {
		if u, re, ok := strings.Cut(v, "="); ok {
			out = append(out, webextract.TextRule{URL: u, Regex: re})
		}
	}
	return out
}
