package main

import (
	"fyne.io/fyne/v2/container"
)

// ---------------------------------------------------------------------------
// Main tabs
// ---------------------------------------------------------------------------

func (u *ui) showMain() {
	libTab := container.NewTabItem("Library", u.buildLibraryTab())
	torTab := container.NewTabItem("Torrents", u.buildTorrentsTab())
	mgmtTab := container.NewTabItem("Library Mgmt", u.buildMgmtTab())
	u.tabs = container.NewAppTabs(libTab, torTab, mgmtTab)
	// Poll torrents only when the Torrents tab is visible.
	u.tabs.OnSelected = func(t *container.TabItem) {
		if t == torTab {
			u.startTorrentPoll()
		} else {
			u.stopTorrentPoll()
		}
	}
	u.window.SetContent(u.tabs)
	u.window.SetOnClosed(func() {
		u.stopTorrentPoll()
		u.stopSeasonPoll()
		u.stopNowPlayingPoll()
	})
}

func (u *ui) stopNowPlayingPoll() {
	if u.nowPlayingStop == nil {
		return
	}
	close(u.nowPlayingStop)
	u.nowPlayingStop = nil
}
