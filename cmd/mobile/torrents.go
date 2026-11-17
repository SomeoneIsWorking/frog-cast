package main

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// ---------------------------------------------------------------------------
// Torrents tab
// ---------------------------------------------------------------------------

func (u *ui) buildTorrentsTab() fyne.CanvasObject {
	u.torrentList = widget.NewList(
		func() int { return len(u.torrents) },
		func() fyne.CanvasObject {
			return container.NewVBox(widget.NewLabel(""), widget.NewProgressBar(), widget.NewLabel(""))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			box := o.(*fyne.Container)
			t := u.torrents[i]
			box.Objects[0].(*widget.Label).SetText(truncate(t.Name, 40))
			box.Objects[1].(*widget.ProgressBar).SetValue(t.Progress)
			box.Objects[2].(*widget.Label).SetText(fmt.Sprintf("%s · %s · %s", t.State, fmtSpeed(t.DlSpeed), fmtEta(t.Eta)))
		},
	)

	magnet := widget.NewEntry()
	magnet.SetPlaceHolder("magnet link")
	add := widget.NewButton("Add", func() {
		m := strings.TrimSpace(magnet.Text)
		if m == "" {
			return
		}
		go func() {
			if err := u.client.AddTorrent(m); err != nil {
				fyne.Do(func() { u.fail(err) })
				return
			}
			fyne.Do(func() { magnet.SetText("") })
			u.pollTorrentsOnce()
		}()
	})
	bottom := container.NewBorder(nil, nil, nil, add, magnet)
	return container.NewBorder(nil, bottom, nil, nil, u.torrentList)
}

func (u *ui) startTorrentPoll() {
	if u.torrentStop != nil {
		return
	}
	u.torrentStop = make(chan struct{})
	stop := u.torrentStop
	u.pollTorrentsOnce()
	go func() {
		t := time.NewTicker(3 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				u.pollTorrentsOnce()
			}
		}
	}()
}

func (u *ui) stopTorrentPoll() {
	if u.torrentStop == nil {
		return
	}
	close(u.torrentStop)
	u.torrentStop = nil
}

func (u *ui) pollTorrentsOnce() {
	go func() {
		list, err := u.client.Torrents()
		if err != nil {
			return
		}
		fyne.Do(func() { u.torrents = list; u.torrentList.Refresh() })
	}()
}
