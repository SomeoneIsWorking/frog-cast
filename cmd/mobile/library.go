package main

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
)

// libraryView browses the instance's library: shows, then seasons, then
// episodes; tapping an episode opens the play sheet. Web pages open from here
// too.
type libraryView struct {
	s       *session
	pages   *pageStack
	content fyne.CanvasObject

	shows    []castapi.LibraryShow
	filtered []castapi.LibraryShow
	list     *widget.List
	status   *widget.Label
}

func newLibraryView(s *session) *libraryView {
	v := &libraryView{s: s}

	v.list = widget.NewList(
		func() int { return len(v.filtered) },
		func() fyne.CanvasObject { return newRow(theme.FolderIcon()) },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			show := v.filtered[i]
			title := show.Name
			if show.Year > 0 {
				title = fmt.Sprintf("%s (%d)", show.Name, show.Year)
			}
			setRow(o, title, countText(show))
		},
	)
	v.list.OnSelected = func(i widget.ListItemID) {
		v.list.Unselect(i)
		v.openShow(v.filtered[i])
	}

	search := widget.NewEntry()
	search.SetPlaceHolder("Search shows")
	search.ActionItem = widget.NewIcon(theme.SearchIcon())
	search.OnChanged = func(q string) { v.filter(q) }

	refresh := widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), v.reload)
	refresh.Importance = widget.LowImportance

	v.status = caption("Loading…")
	v.status.Alignment = fyne.TextAlignCenter

	web := widget.NewButtonWithIcon("Web page", theme.MediaVideoIcon(), func() {
		v.pages.push("Web page", s.web.content)
	})
	web.Importance = widget.LowImportance

	root := container.NewBorder(
		container.NewBorder(nil, nil, nil, container.NewHBox(web, refresh), search),
		v.status, nil, nil, v.list)
	v.pages = newPageStack(root)
	v.content = v.pages.content
	v.reload()
	return v
}

func (v *libraryView) reload() {
	v.status.SetText("Loading…")
	v.status.Show()
	go func() {
		tree, err := v.s.client.LibraryTree()
		fyne.Do(func() {
			if err != nil {
				v.status.SetText("Couldn't load the library: " + err.Error())
				return
			}
			v.shows = tree.Shows
			v.filter("")
			if len(v.shows) == 0 {
				v.status.SetText("The library is empty. Set its folder in the desktop app.")
			} else {
				v.status.Hide()
			}
		})
	}()
}

func (v *libraryView) filter(query string) {
	query = strings.ToLower(strings.TrimSpace(query))
	v.filtered = v.filtered[:0]
	for _, show := range v.shows {
		if query == "" || strings.Contains(strings.ToLower(show.Name), query) {
			v.filtered = append(v.filtered, show)
		}
	}
	v.list.Refresh()
}

// openShow goes to the show's seasons, or straight to its episodes when it
// has only one season.
func (v *libraryView) openShow(show castapi.LibraryShow) {
	if len(show.Seasons) == 1 {
		v.openSeason(show, show.Seasons[0])
		return
	}
	list := widget.NewList(
		func() int { return len(show.Seasons) },
		func() fyne.CanvasObject { return newRow(theme.FolderIcon()) },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			season := show.Seasons[i]
			setRow(o, season.Name, plural(len(season.Episodes), "episode"))
		},
	)
	list.OnSelected = func(i widget.ListItemID) {
		list.Unselect(i)
		v.openSeason(show, show.Seasons[i])
	}
	v.pages.push(show.Name, list)
}

func (v *libraryView) openSeason(show castapi.LibraryShow, season castapi.LibrarySeason) {
	list := widget.NewList(
		func() int { return len(season.Episodes) },
		func() fyne.CanvasObject { return newRow(theme.MediaVideoIcon()) },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			ep := season.Episodes[i]
			setRow(o, episodeTitle(ep), episodeNotes(ep))
		},
	)
	list.OnSelected = func(i widget.ListItemID) {
		list.Unselect(i)
		ep := season.Episodes[i]
		v.s.showPlaySheet(ep.Path, show.Name+" · "+episodeTitle(ep))
	}
	title := show.Name
	if len(show.Seasons) > 1 {
		title = season.Name
	}
	v.pages.push(title, list)
}

func episodeTitle(ep castapi.LibraryEpisode) string {
	if ep.Episode == 0 {
		return ep.Name
	}
	title := fmt.Sprintf("E%02d", ep.Episode)
	if ep.EpisodeName != "" {
		title += " · " + ep.EpisodeName
	}
	return title
}

func episodeNotes(ep castapi.LibraryEpisode) string {
	switch {
	case ep.Translated:
		return "Subtitles · translated"
	case ep.HasSubtitles:
		return "Subtitles"
	default:
		return "No subtitles"
	}
}

func countText(show castapi.LibraryShow) string {
	episodes := 0
	for _, s := range show.Seasons {
		episodes += len(s.Episodes)
	}
	if len(show.Seasons) == 1 {
		return plural(episodes, "episode")
	}
	return plural(len(show.Seasons), "season") + " · " + plural(episodes, "episode")
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

// newRow is a list row: icon, title, a line of detail and a chevron.
func newRow(icon fyne.Resource) fyne.CanvasObject {
	title := widget.NewLabel("")
	title.Truncation = fyne.TextTruncateEllipsis
	detail := widget.NewLabel("")
	detail.Importance = widget.LowImportance
	detail.Truncation = fyne.TextTruncateEllipsis
	return container.NewBorder(nil, nil, widget.NewIcon(icon), widget.NewIcon(theme.NavigateNextIcon()),
		container.NewVBox(title, detail))
}

func setRow(o fyne.CanvasObject, title, detail string) {
	texts := o.(*fyne.Container).Objects[0].(*fyne.Container).Objects
	texts[0].(*widget.Label).SetText(title)
	texts[1].(*widget.Label).SetText(detail)
}

// showPlaySheet picks the target, audio, subtitles and quality for an item
// and starts it.
func (s *session) showPlaySheet(id, title string) {
	go func() {
		info, err := s.client.TrackInfo(id)
		fyne.Do(func() {
			if err != nil {
				s.ui.fail(err)
				return
			}
			s.playSheet(id, title, info)
		})
	}()
}

func (s *session) playSheet(id, title string, info *castapi.TrackDisplayInfo) {
	target := s.targets.newSelect()

	var audioLabels []string
	for _, a := range info.AudioTracks {
		audioLabels = append(audioLabels, trackLabel(a.Index, a.Language))
	}
	audio := widget.NewSelect(audioLabels, nil)
	if len(audioLabels) > 0 {
		audio.SetSelectedIndex(0)
	}

	subs := subtitleChoices(info, "")
	subtitle := widget.NewSelect(subs.labels, nil)
	// A subtitle file beside the media (or its translation) is the usual pick.
	subtitle.SetSelectedIndex(0)
	if info.NearSubtitle != "" {
		subtitle.SetSelectedIndex(subs.index("external:" + info.NearSubtitle))
	}

	quality := widget.NewSelect(qualityLabels, nil)
	quality.PlaceHolder = "Instance default"

	items := []*widget.FormItem{widget.NewFormItem("Cast to", target)}
	if len(audioLabels) > 1 {
		items = append(items, widget.NewFormItem("Audio", audio))
	}
	items = append(items,
		widget.NewFormItem("Subtitles", subtitle),
		widget.NewFormItem("Quality", quality),
	)

	form := dialog.NewForm(title, "Play", "Cancel", items, func(ok bool) {
		if !ok {
			return
		}
		opts := castapi.PlayOptions{}
		if i := audio.SelectedIndex(); i >= 0 {
			opts.AudioTrack = info.AudioTracks[i].Index
		}
		if i := subtitle.SelectedIndex(); i >= 0 {
			opts.SubtitlePath = subs.paths[i]
		}
		if i := quality.SelectedIndex(); i >= 0 {
			opts.Quality = &qualityValues[i]
		}
		host := s.targets.selected()
		go func() {
			_, err := s.client.Play(id, host, opts)
			fyne.Do(func() {
				if err != nil {
					s.ui.fail(err)
					return
				}
				s.showPlaying()
			})
		}()
	}, s.ui.window)
	form.Resize(fyne.NewSize(s.ui.window.Canvas().Size().Width-16, form.MinSize().Height))
	form.Show()
}

func trackLabel(index int, language string) string {
	if language == "" {
		return fmt.Sprintf("Track %d", index)
	}
	return fmt.Sprintf("%s (track %d)", language, index)
}

// pageStack shows a root page and the pages pushed over it, with a back
// button and title above any pushed page.
type pageStack struct {
	content *fyne.Container
	body    *fyne.Container
	header  *fyne.Container
	title   *widget.Label
	pages   []fyne.CanvasObject
	titles  []string
}

func newPageStack(root fyne.CanvasObject) *pageStack {
	p := &pageStack{pages: []fyne.CanvasObject{root}, titles: []string{""}}
	p.title = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	p.title.Truncation = fyne.TextTruncateEllipsis
	back := widget.NewButtonWithIcon("", theme.NavigateBackIcon(), p.back)
	back.Importance = widget.LowImportance
	p.header = container.NewBorder(nil, nil, back, nil, p.title)
	p.body = container.NewStack(root)
	p.content = container.NewBorder(p.header, nil, nil, nil, p.body)
	p.show()
	return p
}

func (p *pageStack) push(title string, page fyne.CanvasObject) {
	p.pages = append(p.pages, page)
	p.titles = append(p.titles, title)
	p.show()
}

func (p *pageStack) back() {
	if len(p.pages) > 1 {
		p.pages = p.pages[:len(p.pages)-1]
		p.titles = p.titles[:len(p.titles)-1]
		p.show()
	}
}

func (p *pageStack) show() {
	top := len(p.pages) - 1
	p.body.Objects = []fyne.CanvasObject{p.pages[top]}
	p.body.Refresh()
	p.title.SetText(p.titles[top])
	p.header.Hidden = top == 0
	p.content.Refresh()
}
