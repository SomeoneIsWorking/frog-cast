package main

import (
	"fmt"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"wails-cast/pkg/castapi"
)

const prefLanguage = "translateLanguage"

// manageView runs the instance's library upkeep: rescan, identify shows on
// TMDB, organize files, and translate a season's subtitles.
type manageView struct {
	s       *session
	content fyne.CanvasObject

	tree    *castapi.LibraryScanResult
	seasons []seasonRef
	list    *widget.List
	status  *widget.Label

	identify *widget.Button
	organize *widget.Button

	progress     *widget.ProgressBar
	progressText *widget.Label
	progressBox  fyne.CanvasObject
	stopSeason   chan struct{}
}

type seasonRef struct {
	show   string
	season castapi.LibrarySeason
}

func newManageView(s *session) *manageView {
	v := &manageView{s: s}

	scan := widget.NewButtonWithIcon("Rescan", theme.ViewRefreshIcon(), v.scan)
	v.identify = widget.NewButtonWithIcon("Identify", theme.SearchIcon(), v.runIdentify)
	v.organize = widget.NewButtonWithIcon("Organize", theme.FolderIcon(), v.previewOrganize)
	v.identify.Disable()
	v.organize.Disable()

	v.list = widget.NewList(
		func() int { return len(v.seasons) },
		func() fyne.CanvasObject {
			title := widget.NewLabel("")
			title.Truncation = fyne.TextTruncateEllipsis
			detail := caption("")
			detail.Wrapping = fyne.TextWrapOff
			translate := widget.NewButtonWithIcon("", theme.DocumentIcon(), nil)
			return container.NewBorder(nil, nil, nil, translate, container.NewVBox(title, detail))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			ref := v.seasons[i]
			box := o.(*fyne.Container)
			texts := box.Objects[0].(*fyne.Container).Objects
			texts[0].(*widget.Label).SetText(ref.show + " · " + ref.season.Name)
			translated := 0
			for _, e := range ref.season.Episodes {
				if e.Translated {
					translated++
				}
			}
			texts[1].(*widget.Label).SetText(fmt.Sprintf("%s · %d translated", plural(len(ref.season.Episodes), "episode"), translated))
			box.Objects[1].(*widget.Button).OnTapped = func() { v.promptTranslate(ref) }
		},
	)
	v.list.OnSelected = func(i widget.ListItemID) { v.list.Unselect(i) }

	v.status = caption("Loading…")
	v.status.Alignment = fyne.TextAlignCenter

	v.progress = widget.NewProgressBar()
	v.progressText = caption("")
	cancel := widget.NewButtonWithIcon("", theme.CancelIcon(), func() {
		go func() {
			if err := s.client.SeasonCancel(); err != nil {
				fyne.Do(func() { s.ui.fail(err) })
			}
		}()
	})
	cancel.Importance = widget.LowImportance
	v.progressBox = container.NewBorder(nil, nil, nil, cancel, container.NewVBox(v.progressText, v.progress))
	v.progressBox.Hide()

	top := container.NewVBox(
		container.NewGridWithColumns(3, scan, v.identify, v.organize),
		caption("Translate a season's subtitles with the button beside it."),
	)
	v.content = container.NewPadded(container.NewBorder(top, container.NewVBox(v.status, v.progressBox), nil, nil, v.list))
	v.scan()
	v.followSeason()
	return v
}

func (v *manageView) setTree(tree *castapi.LibraryScanResult) {
	v.tree = tree
	v.seasons = v.seasons[:0]
	for _, show := range tree.Shows {
		for _, season := range show.Seasons {
			v.seasons = append(v.seasons, seasonRef{show: show.Name, season: season})
		}
	}
	v.list.Refresh()
	v.identify.Enable()
	v.organize.Enable()
	if len(v.seasons) == 0 {
		v.status.SetText("No shows in the library.")
		v.status.Show()
	} else {
		v.status.Hide()
	}
}

func (v *manageView) scan() {
	v.status.SetText("Scanning…")
	v.status.Show()
	go func() {
		tree, err := v.s.client.LibraryTree()
		fyne.Do(func() {
			if err != nil {
				v.status.SetText("Scan failed: " + err.Error())
				return
			}
			v.setTree(tree)
		})
	}()
}

func (v *manageView) runIdentify() {
	if v.tree == nil {
		return
	}
	v.identify.Disable()
	v.status.SetText("Looking up shows on TMDB…")
	v.status.Show()
	tree := *v.tree
	go func() {
		out, err := v.s.client.Identify(tree)
		fyne.Do(func() {
			v.identify.Enable()
			if err != nil {
				v.status.Hide()
				v.s.ui.fail(err)
				return
			}
			v.setTree(out)
		})
	}()
}

func (v *manageView) previewOrganize() {
	if v.tree == nil {
		return
	}
	tree := *v.tree
	go func() {
		plan, err := v.s.client.OrganizePreview(tree)
		fyne.Do(func() {
			if err != nil {
				v.s.ui.fail(err)
				return
			}
			v.showOrganize(plan)
		})
	}()
}

func (v *manageView) showOrganize(plan []castapi.OrganizeMove) {
	if len(plan) == 0 {
		dialog.ShowInformation("Organize", "Everything is already in place.", v.s.ui.window)
		return
	}
	list := widget.NewList(
		func() int { return len(plan) },
		func() fyne.CanvasObject { l := widget.NewLabel(""); l.Wrapping = fyne.TextWrapWord; return l },
		func(i widget.ListItemID, o fyne.CanvasObject) { o.(*widget.Label).SetText(plan[i].Description) },
	)
	size := v.s.ui.window.Canvas().Size()
	d := dialog.NewCustomConfirm(fmt.Sprintf("Move %s?", plural(len(plan), "file")), "Move", "Cancel", list, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			err := v.s.client.OrganizeExecute(plan)
			fyne.Do(func() {
				if err != nil {
					v.s.ui.fail(err)
					return
				}
				v.scan()
			})
		}()
	}, v.s.ui.window)
	d.Resize(fyne.NewSize(size.Width-16, size.Height*0.7))
	d.Show()
}

func (v *manageView) promptTranslate(ref seasonRef) {
	prefs := v.s.ui.fyneApp.Preferences()
	language := widget.NewEntry()
	language.SetText(prefs.StringWithFallback(prefLanguage, "Turkish"))
	d := dialog.NewForm("Translate "+ref.season.Name, "Start", "Cancel",
		[]*widget.FormItem{widget.NewFormItem("Into", language)},
		func(ok bool) {
			lang := strings.TrimSpace(language.Text)
			if !ok || lang == "" {
				return
			}
			prefs.SetString(prefLanguage, lang)
			paths := make([]string, 0, len(ref.season.Episodes))
			for _, e := range ref.season.Episodes {
				paths = append(paths, e.Path)
			}
			go func() {
				err := v.s.client.TranslateSeason(ref.show, ref.season.Name, paths, lang)
				fyne.Do(func() {
					if err != nil {
						v.s.ui.fail(err)
						return
					}
					v.followSeason()
				})
			}()
		}, v.s.ui.window)
	d.Resize(fyne.NewSize(v.s.ui.window.Canvas().Size().Width-16, d.MinSize().Height))
	d.Show()
}

// followSeason shows a running season translation until it ends.
func (v *manageView) followSeason() {
	v.stopSeasonPoll()
	stop := make(chan struct{})
	v.stopSeason = stop
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			st, err := v.s.client.SeasonStatus()
			running := err == nil && st.Status == "running"
			fyne.Do(func() {
				if !running {
					v.progressBox.Hide()
					return
				}
				v.progressText.SetText(fmt.Sprintf("Translating %s · %s", st.SeasonName, st.Message))
				if st.TotalEpisodes > 0 {
					v.progress.SetValue(float64(st.CurrentEpisode) / float64(st.TotalEpisodes))
				}
				v.progressBox.Show()
			})
			if !running {
				return
			}
			select {
			case <-stop:
				return
			case <-t.C:
			}
		}
	}()
}

func (v *manageView) stopSeasonPoll() {
	if v.stopSeason != nil {
		close(v.stopSeason)
		v.stopSeason = nil
	}
}
