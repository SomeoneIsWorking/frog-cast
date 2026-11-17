package remote

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"wails-cast/pkg/filehelper"
	"wails-cast/pkg/hls"
	"wails-cast/pkg/mix"

	"github.com/pkg/errors"
)

type TrackManager struct {
	FileDownloader     FileDownloader
	cacheChannel       chan int
	Manifest           *hls.TrackPlaylist
	ManifestURL        *url.URL
	Folder             string
	Cache              bool
	TrackType          string
	TrackIndex         int
	StorageDirectory   string
	DownloadStatus     string
	DownloadedSegments []bool
	cancelDownload     context.CancelFunc

	initMu   sync.Mutex
	initData []byte
}

type DownloadStatusQeuryResponse struct {
	Status   string
	Segments []bool
}

func (this *TrackManager) StopDownload() error {
	if this.cancelDownload != nil {
		this.cancelDownload()
		this.cancelDownload = nil
	}
	this.DownloadStatus = "STOPPED"
	this.statusUpdate()
	return nil
}

func (this *TrackManager) StartDownload() error {
	if this.DownloadStatus == "INPROGRESS" {
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	this.cancelDownload = cancel
	this.DownloadStatus = "INPROGRESS"
	this.statusUpdate()

	go func() {
		defer cancel()
		for i := range this.Manifest.Segments {
			select {
			case <-ctx.Done():
				return
			default:
				if this.DownloadedSegments[i] {
					continue
				}

				_, err := this.GetSegment(ctx, i)
				if err != nil {
					// Check if it was canceled
					select {
					case <-ctx.Done():
						return
					default:
						// Log error but continue or stop? Let's stop for now on real errors
						this.DownloadStatus = "ERROR"
						this.statusUpdate()
						return
					}
				}
			}
		}
		this.DownloadStatus = "COMPLETED"
		this.statusUpdate()
	}()

	return nil
}

func (this *TrackManager) GetDownloadStatus() *DownloadStatusQeuryResponse {
	return &DownloadStatusQeuryResponse{
		Status:   this.DownloadStatus,
		Segments: this.DownloadedSegments,
	}
}

func (this *TrackManager) StopAndClear() error {
	this.StopDownload()
	err := os.RemoveAll(this.Folder)
	if err != nil {
		return errors.Wrapf(err, "failed to remove folder: %s", this.Folder)
	}
	err = os.MkdirAll(this.Folder, 0755)
	if err != nil {
		return errors.Wrapf(err, "failed to recreate folder: %s", this.Folder)
	}

	this.DownloadedSegments = make([]bool, len(this.Manifest.Segments))
	this.DownloadStatus = "IDLE"
	this.statusUpdate()
	return nil
}

func (this *TrackManager) GetDuration() float64 {
	var totalDuration float64
	for _, segment := range this.Manifest.Segments {
		totalDuration += segment.Duration
	}
	return totalDuration
}

func NewTrackManager(
	fileDownloader FileDownloader,
	manifest *hls.TrackPlaylist,
	manifestURL *url.URL,
	folder string,
	cache bool,
	trackType string,
	trackIndex int,
	storageDirectory string,
	cacheChannel chan int,
) *TrackManager {
	downloaded := computeDownloadedSegments(folder, len(manifest.Segments))
	status := "IDLE"
	allDownloaded := true
	for _, d := range downloaded {
		if !d {
			allDownloaded = false
			break
		}
	}
	if allDownloaded {
		status = "COMPLETED"
	}

	return &TrackManager{
		FileDownloader:     fileDownloader,
		Manifest:           manifest,
		ManifestURL:        manifestURL,
		Folder:             folder,
		Cache:              cache,
		TrackType:          trackType,
		TrackIndex:         trackIndex,
		StorageDirectory:   storageDirectory,
		DownloadedSegments: downloaded,
		cacheChannel:       cacheChannel,
		DownloadStatus:     status,
	}
}

func computeDownloadedSegments(folder string, total int) []bool {
	downloaded := make([]bool, total)
	for i := range total {
		segmentPath := getSegmentPath(folder, i)
		if filehelper.Exists(segmentPath) {
			downloaded[i] = true
		}
	}
	return downloaded
}

func (this *TrackManager) downloadSegment(ctx context.Context, segmentIndex int) ([]byte, error) {
	url, err := this.resolveSegmentURL(segmentIndex)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to resolve segment URL for index: %d", segmentIndex)
	}
	data, err := this.FileDownloader.DownloadFile(ctx, url)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download segment: %s", url)
	}

	// Fragmented-MP4 tracks keep the codec setup in a separate init segment
	// (#EXT-X-MAP). A media segment alone can't be decoded, so prepend the
	// init segment to make every stored segment self-contained for ffmpeg.
	initData, err := this.getInitSegment(ctx)
	if err != nil {
		return nil, err
	}
	if initData != nil {
		data = append(append(make([]byte, 0, len(initData)+len(data)), initData...), data...)
	}

	return data, nil
}

// getInitSegment downloads (once) and returns the track's #EXT-X-MAP init
// segment, or nil when the track has none (MPEG-TS segments).
func (this *TrackManager) getInitSegment(ctx context.Context) ([]byte, error) {
	m := this.Manifest.Map
	if m == nil || m.URI == "" {
		return nil, nil
	}
	this.initMu.Lock()
	defer this.initMu.Unlock()
	if this.initData != nil {
		return this.initData, nil
	}
	ref, err := url.Parse(m.URI)
	if err != nil {
		return nil, errors.Wrapf(err, "invalid init segment URI: %s", m.URI)
	}
	initURL := this.ManifestURL.ResolveReference(ref)
	data, err := this.FileDownloader.DownloadFile(ctx, initURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to download init segment: %s", initURL)
	}
	if br := m.ByteRange; br != nil {
		if br.Offset < 0 || br.Offset+br.Length > int64(len(data)) {
			return nil, fmt.Errorf("init segment byte range %d@%d out of bounds (%d bytes)", br.Length, br.Offset, len(data))
		}
		data = data[br.Offset : br.Offset+br.Length]
	}
	this.initData = data
	return data, nil
}

// probe checks that the first segment not yet on disk still downloads.
func (this *TrackManager) probe(ctx context.Context) error {
	for i, done := range this.DownloadedSegments {
		if done {
			continue
		}
		url, err := this.resolveSegmentURL(i)
		if err != nil {
			return err
		}
		return this.FileDownloader.Probe(ctx, url)
	}
	return nil
}

func (this *TrackManager) resolveSegmentURL(segmentIndex int) (*url.URL, error) {
	segment := this.Manifest.Segments[segmentIndex]
	resolvedUrl := this.ManifestURL.ResolveReference(segment.URI)
	return resolvedUrl, nil
}

func (this *TrackManager) GetSegment(ctx context.Context, segmentIndex int) (*mix.FileOrBuffer, error) {
	cachePath := getSegmentPath(this.Folder, segmentIndex)

	if _, err := os.Stat(cachePath); err == nil {
		return mix.File(cachePath), nil
	}

	data, err := this.downloadSegment(ctx, segmentIndex)
	if err != nil {
		return nil, err
	}

	if !this.Cache {
		return mix.Buffer(data), nil
	}

	if filehelper.WriteFile(cachePath, data) == nil {
		this.DownloadedSegments[segmentIndex] = true
		this.statusUpdate()
	}
	return mix.File(cachePath), nil
}

func (this *TrackManager) statusUpdate() {
	select {
	case this.cacheChannel <- 0:
	default:
	}
}

func getSegmentPath(folder string, segmentIndex int) string {
	return filepath.Join(folder, fmt.Sprintf("segment_%d_raw.ts", segmentIndex))
}
