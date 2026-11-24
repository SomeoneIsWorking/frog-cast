package remote

import (
	"context"

	"frog-cast/pkg/filehelper"
	_logger "frog-cast/pkg/logger"
)

// aheadSeconds is how much of a track is kept cached past the segment the TV
// last asked for, so a slow source doesn't stall playback.
const aheadSeconds = 120

// segmentFetch is one download of a segment into the cache, shared by every
// caller that wants it meanwhile.
type segmentFetch struct {
	done chan struct{}
	err  error
}

// fetchToCache downloads segment i into the cache once, however many callers
// ask for it at the same time. The download outlives a caller that gives up.
func (this *TrackManager) fetchToCache(ctx context.Context, i int) error {
	this.fetchMu.Lock()
	if f, ok := this.fetching[i]; ok {
		this.fetchMu.Unlock()
		select {
		case <-f.done:
			return f.err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	f := &segmentFetch{done: make(chan struct{})}
	if this.fetching == nil {
		this.fetching = map[int]*segmentFetch{}
	}
	this.fetching[i] = f
	this.fetchMu.Unlock()

	data, err := this.downloadSegment(context.WithoutCancel(ctx), i)
	if err == nil {
		err = filehelper.WriteFile(getSegmentPath(this.Folder, i), data)
	}

	this.fetchMu.Lock()
	delete(this.fetching, i)
	if err == nil {
		this.DownloadedSegments[i] = true
	}
	this.fetchMu.Unlock()
	f.err = err
	close(f.done)
	if err == nil {
		this.statusUpdate()
	}
	return err
}

// ReadAhead caches the aheadSeconds of segments after index in the background.
// Each call re-aims the window, so a seek moves it.
func (this *TrackManager) ReadAhead(index int) {
	if !this.Cache {
		return
	}
	this.fetchMu.Lock()
	this.aheadFrom = index + 1
	running := this.aheadRunning
	this.aheadRunning = true
	this.fetchMu.Unlock()
	if !running {
		go this.readAhead()
	}
}

func (this *TrackManager) readAhead() {
	for {
		this.fetchMu.Lock()
		next := this.nextMissingAhead()
		if next < 0 {
			this.aheadRunning = false
		}
		this.fetchMu.Unlock()
		if next < 0 {
			return
		}
		if err := this.fetchToCache(context.Background(), next); err != nil {
			// Stop on a failing source; the TV's next request re-aims and retries.
			_logger.Logger.Warn("Read-ahead stopped", "type", this.TrackType, "segment", next, "error", err)
			this.fetchMu.Lock()
			this.aheadRunning = false
			this.fetchMu.Unlock()
			return
		}
	}
}

// nextMissingAhead is the first uncached segment in the read-ahead window, or
// -1. Called with fetchMu held.
func (this *TrackManager) nextMissingAhead() int {
	covered := 0.0
	for i := this.aheadFrom; i < len(this.Manifest.Segments) && covered < aheadSeconds; i++ {
		if !this.DownloadedSegments[i] {
			return i
		}
		covered += this.Manifest.Segments[i].Duration
	}
	return -1
}
