package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"frog-cast/pkg/castapi"
	"frog-cast/pkg/remote"
)

func dialogue(lines ...string) string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for i, line := range lines {
		fmt.Fprintf(&b, "00:00:%02d.000 --> 00:00:%02d.500\n%s\n\n", i, i, line)
	}
	return b.String()
}

// Pages serve subtitles under opaque tokens and name audio only by NAME; the
// controller should see languages.
func TestExtractedPageTracksAreNamedForPeople(t *testing.T) {
	a, _ := newTestApp(t)
	a.RemoteManager = remote.NewManager(true)
	cdn := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXTINF:6.0,\nseg0.ts\n#EXT-X-ENDLIST\n"))
	}))
	defer cdn.Close()

	turkish := dialogue("Affedersiniz, affedersiniz.", "Hayır, kalkma. Burada kal.", "Adın ne senin?", "Bunu neden yaptığını bilmiyorum ama devam etmelisin.")
	english := dialogue("I'm sorry. I'm sorry.", "No, stay. What's your name?", "I don't know why you did that, but you have to keep going.")
	rec := postJSON(t, NewHTTPServer(a).handleExtracted, "/extracted", castapi.ExtractedPage{
		PageURL:     "https://example.test/watch/44/",
		Title:       "Film 44",
		ManifestURL: cdn.URL + "/44/master.m3u8",
		Manifest: "#EXTM3U\n" +
			`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="a",NAME="Turkish",DEFAULT=YES,URI="aud1.m3u8"` + "\n" +
			`#EXT-X-MEDIA:TYPE=AUDIO,GROUP-ID="a",NAME="Original Audio",DEFAULT=NO,URI="aud2.m3u8"` + "\n" +
			"#EXT-X-STREAM-INF:BANDWIDTH=3000000,RESOLUTION=1280x720,AUDIO=\"a\"\nv/720.m3u8\n",
		Subtitles: []castapi.PageSubtitle{
			{Label: "Cd62-UWUUXmwzn39Jzjgu6QcCi2XE7Yuv", Content: turkish},
			{Label: "C00l0YYId9YpLTulr89EVaXv8Vyo", Content: english},
			{Label: "ECh_vefzgz-iPu_qhrYh93jziEHR", Content: english},
		},
	})
	var info castapi.TrackDisplayInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	var audio []string
	for _, track := range info.AudioTracks {
		audio = append(audio, track.Language)
	}
	if strings.Join(audio, ",") != "Turkish,Original Audio" {
		t.Fatalf("audio = %q", audio)
	}
	var subs []string
	for _, s := range info.SubtitleTracks {
		subs = append(subs, s.Label)
	}
	if got := strings.Join(subs, ","); !strings.Contains(got, "Page: Turkish,Page: English,Page: English 2") {
		t.Fatalf("subtitles = %s", got)
	}
}
