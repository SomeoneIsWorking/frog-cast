package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"wails-cast/pkg/castapi"
	"wails-cast/pkg/options"
)

// ---------------------------------------------------------------------------
// Library tab
// ---------------------------------------------------------------------------

func (u *ui) buildLibraryTab() fyne.CanvasObject {
	u.deviceSel = widget.NewSelect([]string{"Local"}, func(string) {})
	u.deviceSel.SetSelectedIndex(0)

	u.libraryList = widget.NewList(
		func() int { return len(u.libraryItems) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(u.libraryItems[i].Name)
		},
	)
	u.libraryList.OnSelected = func(i widget.ListItemID) {
		u.libraryList.Unselect(i)
		if i < 0 || i >= len(u.libraryItems) {
			return
		}
		u.showPlayDialog(u.libraryItems[i])
	}

	refresh := widget.NewButton("Refresh", func() {
		go u.reloadLibrary()
		go u.refreshDevices()
	})
	disconnect := widget.NewButton("Disconnect", func() { u.showConnect() })

	top := container.NewVBox(
		container.NewHBox(disconnect, refresh),
		widget.NewLabel("Output device:"),
		u.deviceSel,
	)

	go u.reloadLibrary()
	go u.refreshDevices()

	return container.NewBorder(top, nil, nil, nil, u.libraryList)
}

func (u *ui) reloadLibrary() {
	items, err := u.client.Library()
	if err != nil {
		fyne.Do(func() { u.fail(err) })
		return
	}
	fyne.Do(func() { u.libraryItems = items; u.libraryList.Refresh() })
}

func (u *ui) refreshDevices() {
	devs, err := u.client.Devices()
	if err != nil {
		fyne.Do(func() { u.fail(err) })
		return
	}
	u.devicesMu.Lock()
	u.devices = devs
	u.devicesMu.Unlock()

	opts := []string{"Local"}
	for _, d := range devs {
		if d.Host == "local" {
			continue
		}
		opts = append(opts, d.Name)
	}
	fyne.Do(func() {
		u.deviceSel.Options = opts
		u.deviceSel.SetSelectedIndex(0)
		u.deviceSel.Refresh()
	})
}

func (u *ui) selectedDeviceHost() string {
	u.devicesMu.Lock()
	defer u.devicesMu.Unlock()
	idx := u.deviceSel.SelectedIndex()
	if idx <= 0 {
		return "local"
	}
	pos := 0
	for _, d := range u.devices {
		if d.Host == "local" {
			continue
		}
		pos++
		if pos == idx {
			return d.Host
		}
	}
	return "local"
}

func (u *ui) showPlayDialog(item castapi.LibraryItem) {
	go func() {
		info, err := u.client.TrackInfo(item.ID)
		if err != nil {
			fyne.Do(func() { u.fail(err) })
			return
		}

		fyne.Do(func() {
			audioOptions := []string{"Default"}
			for _, a := range info.AudioTracks {
				audioOptions = append(audioOptions, fmt.Sprintf("%d: %s", a.Index, a.Language))
			}
			audioSel := widget.NewSelect(audioOptions, func(string) {})
			audioSel.SetSelectedIndex(0)

			subOptions := []string{"None"}
			for _, s := range info.SubtitleTracks {
				subOptions = append(subOptions, s.Label)
			}
			subSel := widget.NewSelect(subOptions, func(string) {})
			subSel.SetSelectedIndex(0)

			qualityOptions := append([]string{"Default"}, qualityBitrates...)
			qualitySel := widget.NewSelect(qualityOptions, func(string) {})
			qualitySel.SetSelectedIndex(0)

			form := container.NewVBox(
				widget.NewLabel("Audio track"), audioSel,
				widget.NewLabel("Subtitle"), subSel,
				widget.NewLabel("Quality"), qualitySel,
			)

			dialog.ShowCustomConfirm("Play "+item.Name, "Play", "Cancel", form, func(ok bool) {
				if !ok {
					return
				}
				opts := castapi.PlayOptions{}
				if idx := audioSel.SelectedIndex(); idx > 0 && idx-1 < len(info.AudioTracks) {
					opts.AudioTrack = info.AudioTracks[idx-1].Index
				}
				if idx := subSel.SelectedIndex(); idx > 0 && idx-1 < len(info.SubtitleTracks) {
					opts.SubtitlePath = info.SubtitleTracks[idx-1].Path
				}
				if idx := qualitySel.SelectedIndex(); idx > 0 {
					q := qualityOptions[idx]
					opts.Quality = &q
				}
				u.currentSubtitlePath = opts.SubtitlePath
				u.currentSubtitleOpts = options.SubtitleCastOptions{Path: opts.SubtitlePath, FontSize: 24}
				go func() {
					if _, err := u.client.Play(item.ID, u.selectedDeviceHost(), opts); err != nil {
						fyne.Do(func() { u.fail(err) })
						return
					}
					fyne.Do(func() { u.showNowPlaying() })
				}()
			}, u.window)
		})
	}()
}
