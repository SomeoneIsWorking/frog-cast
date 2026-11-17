package main

import (
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"wails-cast/pkg/castapi"
	"wails-cast/pkg/options"
)

const (
	prefBaseURL = "baseURL"
	prefToken   = "token"
)

type ui struct {
	fyneApp fyne.App
	window  fyne.Window
	client  *castapi.Client

	tabs *container.AppTabs

	devicesMu sync.Mutex
	devices   []castapi.RemoteDevice
	deviceSel *widget.Select

	// Library tab
	libraryItems []castapi.LibraryItem
	libraryList  *widget.List

	// Torrents tab
	torrents    []castapi.TorrentStatus
	torrentList *widget.List
	torrentStop chan struct{}

	// Library Mgmt tab
	currentTree      *castapi.LibraryScanResult
	mgmtTree         *widget.Tree
	mgmtIdentifyBtn  *widget.Button
	mgmtPreviewBtn   *widget.Button
	seasonStop       chan struct{}
	seasonBarLabel   *widget.Label
	seasonBarWrapper *fyne.Container

	// Subtitle state
	currentSubtitlePath string
	currentSubtitleOpts options.SubtitleCastOptions

	// Now Playing poller
	nowPlayingStop chan struct{}
}

func main() {
	a := app.NewWithID("io.wailscast.remote")
	w := a.NewWindow("wails-cast remote")
	w.Resize(fyne.NewSize(420, 720))

	u := &ui{fyneApp: a, window: w}
	u.showConnect()

	w.ShowAndRun()
}

func (u *ui) fail(err error) { dialog.ShowError(err, u.window) }
