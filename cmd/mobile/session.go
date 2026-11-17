package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
)

// The tabs, in bar order.
const (
	tabLibrary = iota
	tabPlaying
	tabTorrents
	tabManage
)

// session is the connected app: one instance, its cast targets and the tabs.
type session struct {
	ui      *ui
	name    string
	client  *castapi.Client
	targets *targetPicker

	tabs     *navTabs
	library  *libraryView
	web      *webView
	playing  *nowPlaying
	torrents *torrentsView
	manage   *manageView
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

	s.tabs = newNavTabs(
		navTab{"Library", theme.ListIcon(), s.library.content},
		navTab{"Playing", theme.MediaPlayIcon(), s.playing.content},
		navTab{"Torrents", theme.DownloadIcon(), s.torrents.content},
		navTab{"Manage", theme.SettingsIcon(), s.manage.content},
	)
	s.tabs.OnSelected = func(int) { s.pollVisibleTab() }

	u.session = s
	u.window.SetContent(container.NewBorder(s.appBar(), nil, nil, nil, s.tabs.content))

	s.targets.load()
	u.takeShare()
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
				s.tabs.Select(tabPlaying)
			}
			s.pollVisibleTab()
		})
	}()
}

// appBar names the instance, with a way back to the instance list.
func (s *session) appBar() fyne.CanvasObject {
	dot := canvas.NewCircle(colorSuccess)
	name := widget.NewLabelWithStyle(s.name, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	name.SizeName = theme.SizeNameSubHeadingText
	name.Truncation = fyne.TextTruncateEllipsis
	sub := section("CONNECTED · FROG CAST")
	texts := container.New(layout.NewCustomPaddedVBoxLayout(-14), sub, name)
	disconnect := widget.NewButtonWithIcon("Switch", theme.LogoutIcon(), s.ui.showConnect)
	disconnect.Importance = widget.LowImportance
	mark := container.NewCenter(container.NewGridWrap(fyne.NewSize(10, 10), dot))
	bar := container.NewBorder(nil, nil, mark, container.NewCenter(disconnect), texts)
	return container.New(layout.NewCustomPaddedLayout(8, 4, 14, 8), bar)
}

// showPlaying switches to the Playing tab, e.g. after starting a cast.
func (s *session) showPlaying() {
	s.tabs.Select(tabPlaying)
}

// pollVisibleTab polls only what's on screen.
func (s *session) pollVisibleTab() {
	selected := s.tabs.Selected()
	s.playing.setPolling(selected == tabPlaying)
	s.torrents.setPolling(selected == tabTorrents)
}

func (s *session) close() {
	s.playing.setPolling(false)
	s.torrents.setPolling(false)
	s.manage.stopSeasonPoll()
}
