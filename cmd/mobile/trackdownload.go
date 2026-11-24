package main

import (
	"fmt"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
)

// trackDownload shows and drives the download of one track of a web page
// into the instance's cache. It polls while onScreen says it is visible.
type trackDownload struct {
	s        *session
	pageURL  string
	kind     string
	onScreen func() bool
	track    int
	content  fyne.CanvasObject
	button   *widget.Button
	progress *thinBar
	label    *widget.Label
	status   castapi.DownloadStatus
	polling  bool
}

func newTrackDownload(s *session, kind string, onScreen func() bool) *trackDownload {
	d := &trackDownload{s: s, kind: kind, track: -1, onScreen: onScreen}
	d.button = widget.NewButtonWithIcon("", theme.DownloadIcon(), d.toggle)
	d.progress = newThinBar()
	d.label = note("")
	d.label.SizeName = theme.SizeNameCaptionText
	d.label.Truncation = fyne.TextTruncateEllipsis
	bar := container.NewVBox(layout.NewSpacer(), d.progress, layout.NewSpacer())
	d.content = container.NewBorder(nil, nil, nil, d.button, container.New(layout.NewCustomPaddedVBoxLayout(-6), bar, d.label))
	return d
}

func (d *trackDownload) setTrack(pageURL string, track int) {
	if pageURL == d.pageURL && track == d.track {
		return
	}
	d.pageURL = pageURL
	d.track = track
	d.status = castapi.DownloadStatus{}
	d.show()
	d.refresh()
}

func (d *trackDownload) refresh() {
	client, pageURL, kind, track := d.s.client, d.pageURL, d.kind, d.track
	go func() {
		st, err := client.Download(pageURL, kind, track)
		fyne.Do(func() {
			if track != d.track || pageURL != d.pageURL {
				return
			}
			if err != nil {
				// One missed read must not freeze the bar: keep following.
				d.follow()
				return
			}
			d.status = *st
			d.show()
			d.follow()
		})
	}()
}

func (d *trackDownload) toggle() {
	client, req := d.s.client, castapi.DownloadRequest{
		URL: d.pageURL, Type: d.kind, Track: d.track, Start: d.status.Status != "INPROGRESS",
	}
	d.button.Disable()
	go func() {
		st, err := client.SetDownload(req)
		fyne.Do(func() {
			d.button.Enable()
			if err != nil {
				d.s.ui.fail(err)
				return
			}
			if req.Track == d.track && req.URL == d.pageURL {
				d.status = *st
				d.show()
				d.follow()
			}
		})
	}()
}

// follow polls the download while it runs and the page is on screen.
func (d *trackDownload) follow() {
	if d.polling || d.status.Status != "INPROGRESS" {
		return
	}
	d.polling = true
	go func() {
		time.Sleep(2 * time.Second)
		fyne.Do(func() {
			d.polling = false
			if d.onScreen() {
				d.refresh()
			}
		})
	}()
}

func (d *trackDownload) show() {
	st := d.status
	if st.Total > 0 {
		d.progress.SetValue(float64(st.Done) / float64(st.Total))
	} else {
		d.progress.SetValue(0)
	}
	switch st.Status {
	case "INPROGRESS":
		d.button.SetIcon(theme.MediaStopIcon())
		d.label.SetText(fmt.Sprintf("Downloading · %d of %d segments", st.Done, st.Total))
	case "COMPLETED":
		d.button.SetIcon(theme.ConfirmIcon())
		d.label.SetText(fmt.Sprintf("Downloaded · %d segments", st.Total))
	case "ERROR":
		d.button.SetIcon(theme.DownloadIcon())
		d.label.SetText(fmt.Sprintf("Download failed · %d of %d segments", st.Done, st.Total))
	case "":
		d.button.SetIcon(theme.DownloadIcon())
		d.label.SetText("Checking the cache…")
	default:
		d.button.SetIcon(theme.DownloadIcon())
		d.label.SetText(fmt.Sprintf("%d of %d segments downloaded", st.Done, st.Total))
	}
	d.button.Disable()
	if st.Status != "" && st.Status != "COMPLETED" {
		d.button.Enable()
	}
}
