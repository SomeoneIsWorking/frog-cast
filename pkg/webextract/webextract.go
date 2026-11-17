// Package webextract finds a web page's HLS stream by loading it in a browser
// engine with capture.js injected into every frame. On Linux the engine is a
// WebKitGTK view (no devtools, no Chrome); elsewhere it is Chromium until a
// native host exists. The same script runs in an Android WebView.
package webextract

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

//go:embed capture.js
var captureJS string

// Rules adapt extraction to a site.
type Rules struct {
	// Click lists selectors clicked once they appear in any frame (play
	// overlays). The click goes to the element itself, so it must be the one
	// that handles it.
	Click []string `json:"click,omitempty"`
	// Block lists URL substrings never loaded (anti-debugger and ad scripts).
	Block []string `json:"-"`
	// Intercept lists URL substrings whose response bodies, or frame HTML,
	// go to Options.OnBody.
	Intercept []string `json:"intercept,omitempty"`
}

// Options control one extraction.
type Options struct {
	// Visible shows the page in a window, for someone to click play.
	Visible bool
	// OnBody looks for the stream URL in an intercepted body; "" if it isn't
	// there. It runs on the goroutine that called Extract.
	OnBody func(url, body string) string
	// Trace, when set, sees every message the page's frames report.
	Trace func(Message)
}

// Message is what capture.js reports.
type Message struct {
	Type    string `json:"type"` // frame, click, manifest, manifestUrl, subtitle, body
	URL     string `json:"url"`
	Frame   string `json:"frame"` // URL of the frame that made the request
	Top     bool   `json:"top"`
	Title   string `json:"title"`
	Cookies string `json:"cookies"`
	Body    string `json:"body"`
	Master  bool   `json:"master"`
	Label   string `json:"label"`
}

// Subtitle is a WebVTT file the page loaded.
type Subtitle struct {
	URL     string
	Label   string
	Content string
}

// Result is a page's stream as the browser found it.
type Result struct {
	ManifestURL string
	Manifest    string
	Title       string
	Headers     map[string]string
	Cookies     map[string]string
	Subtitles   []Subtitle
}

// script is capture.js with the rules it reads.
func script(rules Rules) (string, error) {
	config, err := json.Marshal(rules)
	if err != nil {
		return "", err
	}
	return "const config = " + string(config) + ";\n" + captureJS, nil
}

// blockList is a WebKit content-blocker list for rules.Block.
func blockList(rules Rules) string {
	type trigger struct {
		URLFilter string `json:"url-filter"`
	}
	type action struct {
		Type string `json:"type"`
	}
	type entry struct {
		Trigger trigger `json:"trigger"`
		Action  action  `json:"action"`
	}
	entries := []entry{}
	for _, b := range rules.Block {
		entries = append(entries, entry{trigger{".*" + regexpQuote(b) + ".*"}, action{"block"}})
	}
	out, _ := json.Marshal(entries)
	return string(out)
}

// regexpQuote escapes for WebKit's content-blocker regex subset.
func regexpQuote(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune(`\.*+?^$()[]{}|`, r) {
			b.WriteRune('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// collector picks the stream from the messages of one page load.
type collector struct {
	title     string
	master    *Message
	media     *Message
	urlOnly   *Message
	subtitles []Subtitle
	firstSeen time.Time
}

func (c *collector) add(m Message, onBody func(url, body string) string) {
	if m.Top && m.Title != "" {
		c.title = m.Title
	}
	switch m.Type {
	case "manifest":
		if m.Master && c.master == nil {
			c.master = &m
		} else if !m.Master && c.media == nil {
			c.media = &m
		}
	case "manifestUrl":
		if c.urlOnly == nil {
			c.urlOnly = &m
		}
	case "body":
		if onBody == nil || c.urlOnly != nil {
			return
		}
		found := onBody(m.URL, m.Body)
		if found == "" {
			return
		}
		// The frame that received the body is the one that would request it.
		c.urlOnly = &Message{Type: "manifestUrl", URL: resolve(m.URL, found), Frame: m.Frame}
	case "subtitle":
		c.subtitles = append(c.subtitles, Subtitle{URL: m.URL, Label: m.Label, Content: m.Body})
		return
	default:
		return
	}
	if c.firstSeen.IsZero() && (c.master != nil || c.media != nil || c.urlOnly != nil) {
		c.firstSeen = time.Now()
	}
}

func resolve(base, ref string) string {
	b, err := url.Parse(base)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

// settle is how long to keep listening after the first stream, for a master
// playlist after a media one and for subtitles.
const settle = 2 * time.Second

// ready reports whether listening can stop.
func (c *collector) ready(now time.Time) bool {
	if c.master != nil || c.urlOnly != nil {
		return now.Sub(c.firstSeen) >= settle/2
	}
	return !c.firstSeen.IsZero() && now.Sub(c.firstSeen) >= settle
}

// listen collects messages until the stream is known or ctx ends.
func listen(ctx context.Context, pageURL string, messages <-chan Message, opts Options) (*collector, error) {
	c := &collector{}
	tick := time.NewTicker(200 * time.Millisecond)
	defer tick.Stop()
	for !c.ready(time.Now()) {
		select {
		case m := <-messages:
			if opts.Trace != nil {
				opts.Trace(m)
			}
			c.add(m, opts.OnBody)
		case <-tick.C:
		case <-ctx.Done():
			if c.firstSeen.IsZero() {
				return nil, fmt.Errorf("no HLS stream on %s: %w", pageURL, ctx.Err())
			}
			return c, nil
		}
	}
	return c, nil
}

// pick is the stream to cast: a master playlist over a URL a site rule found
// over a media playlist.
func (c *collector) pick() *Message {
	switch {
	case c.master != nil:
		return c.master
	case c.urlOnly != nil:
		return c.urlOnly
	default:
		return c.media
	}
}

// result builds the Result, fetching the playlist when only its URL is known.
// pageCookies are the browser's cookies for the playlist's URL, HttpOnly too.
func (c *collector) result(userAgent string, pageCookies map[string]string) (*Result, error) {
	pick := c.pick()
	if pick == nil {
		return nil, fmt.Errorf("the page loaded no HLS stream")
	}
	headers := map[string]string{"User-Agent": userAgent, "Referer": pick.Frame}
	if frame, err := url.Parse(pick.Frame); err == nil {
		if target, err := url.Parse(pick.URL); err == nil && target.Host != frame.Host {
			headers["Origin"] = frame.Scheme + "://" + frame.Host
		}
	}
	cookies := map[string]string{}
	for k, v := range pageCookies {
		cookies[k] = v
	}
	body := pick.Body
	if body == "" {
		var err error
		if body, err = fetch(pick.URL, headers, cookies); err != nil {
			return nil, fmt.Errorf("loading %s: %w", pick.URL, err)
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(body), "#EXTM3U") {
		return nil, fmt.Errorf("%s is not an HLS playlist", pick.URL)
	}
	return &Result{
		ManifestURL: pick.URL,
		Manifest:    body,
		Title:       c.title,
		Headers:     headers,
		Cookies:     cookies,
		Subtitles:   c.subtitles,
	}, nil
}

func fetch(target string, headers, cookies map[string]string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	for k, v := range cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}
	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status code %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}
