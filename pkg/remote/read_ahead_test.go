package remote

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"frog-cast/pkg/hls"
)

// cdn serves segN.ts after a delay and counts requests per segment.
func cdn(t *testing.T, delay time.Duration) (*httptest.Server, *sync.Map) {
	t.Helper()
	var hits sync.Map
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n, _ := hits.LoadOrStore(r.URL.Path, new(atomic.Int32))
		n.(*atomic.Int32).Add(1)
		time.Sleep(delay)
		fmt.Fprintf(w, "data of %s", r.URL.Path)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func testTrack(t *testing.T, srv *httptest.Server, segments int, seconds float64) *TrackManager {
	t.Helper()
	playlist := &hls.TrackPlaylist{}
	for i := range segments {
		u, _ := url.Parse(fmt.Sprintf("seg%d.ts", i))
		playlist.Segments = append(playlist.Segments, &hls.Segment{Duration: seconds, URI: u})
	}
	base, _ := url.Parse(srv.URL + "/t/index.m3u8")
	return NewTrackManager(FileDownloader{}, playlist, base, t.TempDir(), true, "video", 0, t.TempDir(), nil)
}

func hitsOf(hits *sync.Map, path string) int32 {
	n, ok := hits.Load(path)
	if !ok {
		return 0
	}
	return n.(*atomic.Int32).Load()
}

func TestConcurrentRequestsShareOneDownload(t *testing.T) {
	srv, hits := cdn(t, 50*time.Millisecond)
	track := testTrack(t, srv, 3, 6)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := track.GetSegment(context.Background(), 1); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if n := hitsOf(hits, "/t/seg1.ts"); n != 1 {
		t.Fatalf("segment 1 fetched %d times", n)
	}
}

func TestReadAheadCachesTheWindowAfterTheServedSegment(t *testing.T) {
	srv, hits := cdn(t, 0)
	// 10 s segments: the 120 s window is segments 3..14.
	track := testTrack(t, srv, 30, 10)
	track.ReadAhead(2)
	deadline := time.Now().Add(5 * time.Second)
	for {
		track.fetchMu.Lock()
		running := track.aheadRunning
		track.fetchMu.Unlock()
		if !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("read-ahead still running")
		}
		time.Sleep(10 * time.Millisecond)
	}
	for i := range 30 {
		want := int32(0)
		if i >= 3 && i <= 14 {
			want = 1
		}
		if got := hitsOf(hits, fmt.Sprintf("/t/seg%d.ts", i)); got != want {
			t.Errorf("segment %d fetched %d times, want %d", i, got, want)
		}
	}
	if !track.DownloadedSegments[14] || track.DownloadedSegments[15] {
		t.Fatalf("cached = %v", track.DownloadedSegments)
	}
}
