//go:build !linux || chromium

package webextract

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
)

// binding is the function capture.js posts through in Chromium.
const binding = "__castPost"

// Chromium builds lack the proprietary codecs, and players that check for
// them never request the stream.
const claimCodecs = `
HTMLMediaElement.prototype.canPlayType = (type) => /video|audio/.test(type || "") ? "probably" : "";
if (window.MediaSource) {
	MediaSource.isTypeSupported = (type) => /video|audio/.test(type || "");
}`

// Extract loads pageURL in Chromium (headless unless visible, for someone to
// click play) until it plays an HLS stream or ctx ends.
func Extract(ctx context.Context, pageURL string, rules Rules, opts Options) (*Result, error) {
	js, err := script(rules)
	if err != nil {
		return nil, err
	}
	bin, _ := launcher.LookPath()
	control, err := launcher.New().Bin(bin).Headless(!opts.Visible).
		// One renderer for every frame, so the script and binding reach
		// cross-origin player iframes.
		Set("disable-features", "IsolateOrigins,site-per-process").
		Set("autoplay-policy", "no-user-gesture-required").
		Set("mute-audio").
		Launch()
	if err != nil {
		return nil, fmt.Errorf("starting Chromium: %w", err)
	}
	browser := rod.New().ControlURL(control)
	if err := browser.Connect(); err != nil {
		return nil, fmt.Errorf("connecting to Chromium: %w", err)
	}
	defer browser.Close()

	page, err := browser.Page(proto.TargetCreateTarget{})
	if err != nil {
		return nil, err
	}
	version, err := proto.BrowserGetVersion{}.Call(browser)
	if err != nil {
		return nil, err
	}
	userAgent := strings.Replace(version.UserAgent, "HeadlessChrome", "Chrome", 1)
	messages := make(chan Message, 256)
	go page.EachEvent(func(e *proto.RuntimeBindingCalled) {
		var m Message
		if e.Name != binding || json.Unmarshal([]byte(e.Payload), &m) != nil {
			return
		}
		select {
		case messages <- m:
		default:
		}
	})()

	setup := []func() error{
		func() error { return proto.RuntimeEnable{}.Call(page) },
		func() error { return proto.NetworkEnable{}.Call(page) },
		func() error { return page.SetUserAgent(&proto.NetworkSetUserAgentOverride{UserAgent: userAgent}) },
		func() error { return proto.RuntimeAddBinding{Name: binding}.Call(page) },
		func() error {
			urls := make([]string, 0, len(rules.Block))
			for _, b := range rules.Block {
				urls = append(urls, "*"+b+"*")
			}
			return proto.NetworkSetBlockedURLs{Urls: urls}.Call(page)
		},
		func() error { _, err := page.EvalOnNewDocument(claimCodecs); return err },
		func() error { _, err := page.EvalOnNewDocument(js); return err },
		func() error { return page.Navigate(pageURL) },
	}
	for _, step := range setup {
		if err := step(); err != nil {
			return nil, err
		}
	}

	c, err := listen(ctx, pageURL, messages, opts)
	if err != nil {
		return nil, err
	}
	cookies := map[string]string{}
	if got, err := (proto.NetworkGetCookies{Urls: []string{c.pick().URL}}).Call(page); err == nil {
		for _, cookie := range got.Cookies {
			cookies[cookie.Name] = cookie.Value
		}
	}
	return c.result(userAgent, cookies)
}

// UseHostLoop does nothing: Chromium needs no loop in this process.
func UseHostLoop() {}

// Available reports whether Extract can run in this process.
func Available() bool {
	return true
}

// RunMainLoop runs fn; Chromium needs no loop in this process.
func RunMainLoop(fn func()) {
	fn()
}
