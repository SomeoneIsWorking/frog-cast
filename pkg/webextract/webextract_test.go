package webextract

import (
	"strings"
	"testing"
	"time"
)

func TestCollectorPrefersTheMasterPlaylist(t *testing.T) {
	var c collector
	frame := "https://player.example.test/embed/1"
	c.add(Message{Type: "frame", Top: true, Title: "Film"}, nil)
	c.add(Message{Type: "manifest", URL: "https://cdn.example.test/v/720.m3u8", Frame: frame, Body: "#EXTM3U\n#EXTINF:6,\nseg0.ts\n"}, nil)
	if c.ready(time.Now()) {
		t.Fatal("ready before a master playlist could arrive")
	}
	c.add(Message{Type: "manifest", URL: "https://cdn.example.test/master.m3u8", Frame: frame, Master: true,
		Body: "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\nv/720.m3u8\n"}, nil)
	c.add(Message{Type: "subtitle", URL: "https://cdn.example.test/tr.vtt", Label: "tr", Body: "WEBVTT\n"}, nil)
	if !c.ready(time.Now().Add(settle)) {
		t.Fatal("not ready after the master playlist settled")
	}

	res, err := c.result("UA", map[string]string{"sid": "1"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ManifestURL != "https://cdn.example.test/master.m3u8" || res.Title != "Film" || len(res.Subtitles) != 1 {
		t.Fatalf("result = %+v", res)
	}
	want := map[string]string{"User-Agent": "UA", "Referer": frame, "Origin": "https://player.example.test"}
	for k, v := range want {
		if res.Headers[k] != v {
			t.Fatalf("headers = %v, want %v", res.Headers, want)
		}
	}
}

func TestBlockListEscapesSubstrings(t *testing.T) {
	list := blockList(Rules{Block: []string{"/assets/js/pop.js"}})
	if !strings.Contains(list, `"url-filter":".*/assets/js/pop\\.js.*"`) || !strings.Contains(list, `"type":"block"`) {
		t.Fatalf("block list = %s", list)
	}
}

func TestInterceptedBodyNamesTheStream(t *testing.T) {
	var c collector
	embed := "https://player.example.test/video/abc"
	onBody := func(url, body string) string {
		if strings.Contains(body, "streamUrl") {
			return "/manifests/abc/master.txt"
		}
		return ""
	}
	c.add(Message{Type: "body", URL: embed, Frame: embed, Body: "<html>nothing</html>"}, onBody)
	if !c.firstSeen.IsZero() {
		t.Fatal("a body without the stream counted as found")
	}
	c.add(Message{Type: "body", URL: embed, Frame: embed, Body: `const streamUrl = "/manifests/abc/master.txt";`}, onBody)
	pick := c.pick()
	if pick == nil || pick.URL != "https://player.example.test/manifests/abc/master.txt" || pick.Frame != embed {
		t.Fatalf("pick = %+v", pick)
	}
	if !c.ready(time.Now().Add(settle / 2)) {
		t.Fatal("not ready after an intercepted stream URL")
	}
}
