package main

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
)

// downloadsView shows what the instance is downloading, polled every few
// seconds while on screen: its qBittorrent torrents, which magnet links add
// to, and the web page tracks downloaded into its cache.
type downloadsView struct {
	s        *session
	content  fyne.CanvasObject
	torrents []castapi.TorrentStatus
	pages    []castapi.PageDownload
	list     *widget.List
	empty    fyne.CanvasObject
	stopPoll chan struct{}
}

// downloadRow is one torrent or page track: title, progress, detail and, for
// a page track, a stop or resume button.
type downloadRow struct {
	widget.BaseWidget
	title   *widget.Label
	bar     *thinBar
	detail  *widget.Label
	action  *widget.Button
	content fyne.CanvasObject
}

func newDownloadRow() *downloadRow {
	r := &downloadRow{}
	r.title = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	r.title.Truncation = fyne.TextTruncateEllipsis
	r.bar = newThinBar()
	r.detail = note("")
	r.detail.SizeName = theme.SizeNameCaptionText
	r.detail.Truncation = fyne.TextTruncateEllipsis
	r.action = widget.NewButtonWithIcon("", theme.MediaStopIcon(), nil)
	r.action.Importance = widget.LowImportance
	texts := container.New(layout.NewCustomPaddedVBoxLayout(-6), r.title, container.NewPadded(r.bar), r.detail)
	body := container.NewBorder(nil, nil, nil, container.NewCenter(r.action), texts)
	r.content = container.New(layout.NewCustomPaddedLayout(3, 3, 0, 0), compactCard(body))
	r.ExtendBaseWidget(r)
	return r
}

func (r *downloadRow) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(r.content)
}

func newDownloadsView(s *session) *downloadsView {
	v := &downloadsView{s: s}
	v.list = cardList(
		func() int { return len(v.torrents) + len(v.pages) },
		func() fyne.CanvasObject { return newDownloadRow() },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			r := o.(*downloadRow)
			if i < len(v.torrents) {
				v.showTorrent(r, v.torrents[i])
				return
			}
			v.showPage(r, v.pages[i-len(v.torrents)])
		},
	)
	v.empty = emptyState(theme.DownloadIcon(), "No downloads",
		"Paste a magnet link above, or download a web page's tracks from its Cast options.")

	magnet := widget.NewEntry()
	magnet.SetPlaceHolder("Paste a magnet link")
	add := widget.NewButtonWithIcon("Add", theme.ContentAddIcon(), func() {
		m := strings.TrimSpace(magnet.Text)
		if m == "" {
			return
		}
		go func() {
			err := s.client.AddTorrent(m)
			fyne.Do(func() {
				if err != nil {
					s.ui.fail(err)
					return
				}
				magnet.SetText("")
				v.pollOnce()
			})
		}()
	})
	add.Importance = widget.HighImportance
	top := container.NewBorder(nil, nil, nil, add, magnet)
	v.content = container.NewBorder(container.NewVBox(top, section("ON "+strings.ToUpper(s.name))), nil, nil, nil, container.NewStack(v.list, v.empty))
	return v
}

func (v *downloadsView) showTorrent(r *downloadRow, t castapi.TorrentStatus) {
	r.title.SetText(t.Name)
	r.bar.SetValue(t.Progress)
	r.detail.SetText(fmt.Sprintf("Torrent · %s · %s · %s", t.State, fmtSpeed(t.DlSpeed), fmtEta(t.Eta)))
	r.action.Hide()
}

func (v *downloadsView) showPage(r *downloadRow, d castapi.PageDownload) {
	title := d.Title
	if title == "" {
		title = d.URL
	}
	r.title.SetText(title)
	if d.Total > 0 {
		r.bar.SetValue(float64(d.Done) / float64(d.Total))
	} else {
		r.bar.SetValue(0)
	}
	r.detail.SetText(fmt.Sprintf("%s track %d · %d of %d segments · %s", d.Type, d.Track, d.Done, d.Total, downloadState(d.Status)))
	switch d.Status {
	case "INPROGRESS":
		r.action.SetIcon(theme.MediaStopIcon())
		r.action.Show()
	case "STOPPED", "ERROR":
		r.action.SetIcon(theme.DownloadIcon())
		r.action.Show()
	default:
		r.action.Hide()
	}
	req := castapi.DownloadRequest{URL: d.URL, Type: d.Type, Track: d.Track, Start: d.Status != "INPROGRESS"}
	r.action.OnTapped = func() {
		go func() {
			_, err := v.s.client.SetDownload(req)
			fyne.Do(func() {
				if err != nil {
					v.s.ui.fail(err)
					return
				}
				v.pollOnce()
			})
		}()
	}
}

// downloadState names a download status for people.
func downloadState(status string) string {
	switch status {
	case "INPROGRESS":
		return "downloading"
	case "COMPLETED":
		return "done"
	case "ERROR":
		return "failed"
	default:
		return strings.ToLower(status)
	}
}

func (v *downloadsView) setPolling(on bool) {
	if !on {
		if v.stopPoll != nil {
			close(v.stopPoll)
			v.stopPoll = nil
		}
		return
	}
	if v.stopPoll != nil {
		return
	}
	stop := make(chan struct{})
	v.stopPoll = stop
	v.pollOnce()
	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				v.pollOnce()
			}
		}
	}()
}

// pollOnce reads both lists; one that fails (no qBittorrent configured, an
// older instance without page downloads) shows as empty.
func (v *downloadsView) pollOnce() {
	go func() {
		torrents, _ := v.s.client.Torrents()
		pages, _ := v.s.client.Downloads()
		fyne.Do(func() {
			v.torrents, v.pages = torrents, pages
			if len(torrents)+len(pages) > 0 {
				v.empty.Hide()
			} else {
				v.empty.Show()
			}
			v.list.Refresh()
		})
	}()
}
