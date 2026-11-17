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

// torrentsView adds magnet links to the instance's qBittorrent and shows the
// downloads, polled every few seconds while on screen.
type torrentsView struct {
	s        *session
	content  fyne.CanvasObject
	torrents []castapi.TorrentStatus
	list     *widget.List
	empty    fyne.CanvasObject
	stopPoll chan struct{}
}

func newTorrentsView(s *session) *torrentsView {
	v := &torrentsView{s: s}
	v.list = cardList(
		func() int { return len(v.torrents) },
		func() fyne.CanvasObject {
			name := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			name.Truncation = fyne.TextTruncateEllipsis
			detail := note("")
			detail.SizeName = theme.SizeNameCaptionText
			box := container.New(layout.NewCustomPaddedVBoxLayout(0), name, widget.NewProgressBar(), detail)
			return container.New(layout.NewCustomPaddedLayout(3, 3, 0, 0), compactCard(box))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			t := v.torrents[i]
			box := o.(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*fyne.Container).Objects[0].(*fyne.Container).Objects
			box[0].(*widget.Label).SetText(t.Name)
			box[1].(*widget.ProgressBar).SetValue(t.Progress)
			box[2].(*widget.Label).SetText(fmt.Sprintf("%s · %s · %s", t.State, fmtSpeed(t.DlSpeed), fmtEta(t.Eta)))
		},
	)
	v.empty = emptyState(theme.DownloadIcon(), "No downloads", "Paste a magnet link above to start one.")

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
	v.content = container.NewBorder(container.NewVBox(top, section("DOWNLOADS")), nil, nil, nil, container.NewStack(v.list, v.empty))
	return v
}

func (v *torrentsView) setPolling(on bool) {
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

func (v *torrentsView) pollOnce() {
	go func() {
		list, err := v.s.client.Torrents()
		if err != nil {
			return
		}
		fyne.Do(func() {
			v.torrents = list
			if len(list) > 0 {
				v.empty.Hide()
			} else {
				v.empty.Show()
			}
			v.list.Refresh()
		})
	}()
}
