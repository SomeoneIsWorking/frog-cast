package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
)

// session is the connected app: one instance, its cast targets and the tabs.
type session struct {
	ui      *ui
	name    string
	client  *castapi.Client
	targets *targetPicker

	tabs       *container.AppTabs
	playingTab *container.TabItem
	torrentTab *container.TabItem
	library    *libraryView
	web        *webView
	playing    *nowPlaying
	torrents   *torrentsView
	manage     *manageView
}

func (u *ui) openSession(name string, client *castapi.Client) {
	u.closeSession()
	s := &session{ui: u, name: name, client: client}
	s.targets = newTargetPicker(s)
	s.library = newLibraryView(s)
	s.web = newWebView(s)
	s.playing = newNowPlaying(s)
	s.torrents = newTorrentsView(s)
	s.manage = newManageView(s)

	s.playingTab = container.NewTabItemWithIcon("Playing", theme.MediaPlayIcon(), s.playing.content)
	s.torrentTab = container.NewTabItemWithIcon("Torrents", theme.DownloadIcon(), s.torrents.content)
	s.tabs = container.NewAppTabs(
		container.NewTabItemWithIcon("Library", theme.ListIcon(), s.library.content),
		s.playingTab,
		s.torrentTab,
		container.NewTabItemWithIcon("Manage", theme.SettingsIcon(), s.manage.content),
	)
	s.tabs.SetTabLocation(container.TabLocationBottom)
	s.tabs.OnSelected = func(*container.TabItem) { s.pollVisibleTab() }

	disconnect := widget.NewButtonWithIcon("", theme.LogoutIcon(), u.showConnect)
	disconnect.Importance = widget.LowImportance
	title := widget.NewLabelWithStyle(name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	title.Truncation = fyne.TextTruncateEllipsis
	bar := container.NewBorder(nil, nil, widget.NewIcon(theme.ComputerIcon()), disconnect, title)

	u.session = s
	u.window.SetContent(container.NewBorder(container.NewPadded(bar), nil, nil, nil, s.tabs))

	s.targets.load()
	// Open on what's playing, if anything is.
	go func() {
		st, err := client.State()
		fyne.Do(func() {
			if u.session != s {
				return
			}
			if err != nil {
				u.fail(err)
				return
			}
			if st.Status != "STOPPED" {
				s.tabs.Select(s.playingTab)
			}
			s.pollVisibleTab()
		})
	}()
}

// showPlaying switches to the Playing tab, e.g. after starting a cast.
func (s *session) showPlaying() {
	s.tabs.Select(s.playingTab)
	s.pollVisibleTab()
}

// pollVisibleTab polls only what's on screen.
func (s *session) pollVisibleTab() {
	selected := s.tabs.Selected()
	s.playing.setPolling(selected == s.playingTab)
	s.torrents.setPolling(selected == s.torrentTab)
}

func (s *session) close() {
	s.playing.setPolling(false)
	s.torrents.setPolling(false)
	s.manage.stopSeasonPoll()
}
