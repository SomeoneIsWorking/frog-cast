package main

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"wails-cast/pkg/castapi"
)

// torrentsView adds magnet links to the instance's qBittorrent and shows the
// downloads, polled every few seconds while on screen.
type torrentsView struct {
	s        *session
	content  fyne.CanvasObject
	torrents []castapi.TorrentStatus
	list     *widget.List
	empty    *widget.Label
	stopPoll chan struct{}
}

func newTorrentsView(s *session) *torrentsView {
	v := &torrentsView{s: s}
	v.list = widget.NewList(
		func() int { return len(v.torrents) },
		func() fyne.CanvasObject {
			name := widget.NewLabel("")
			name.Truncation = fyne.TextTruncateEllipsis
			detail := caption("")
			detail.Wrapping = fyne.TextWrapOff
			return container.NewVBox(name, widget.NewProgressBar(), detail)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			t := v.torrents[i]
			box := o.(*fyne.Container).Objects
			box[0].(*widget.Label).SetText(t.Name)
			box[1].(*widget.ProgressBar).SetValue(t.Progress)
			box[2].(*widget.Label).SetText(fmt.Sprintf("%s · %s · %s", t.State, fmtSpeed(t.DlSpeed), fmtEta(t.Eta)))
		},
	)
	v.empty = note("No downloads.")
	v.empty.Alignment = fyne.TextAlignCenter

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
	v.content = container.NewPadded(container.NewBorder(top, nil, nil, nil, container.NewStack(v.list, container.NewCenter(v.empty))))
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
			v.empty.Hidden = len(list) > 0
			v.empty.Refresh()
			v.list.Refresh()
		})
	}()
}
