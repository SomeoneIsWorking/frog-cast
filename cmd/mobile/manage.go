package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"

	"wails-cast/pkg/castapi"
)

// ---------------------------------------------------------------------------
// Library Mgmt tab
// ---------------------------------------------------------------------------

func (u *ui) buildMgmtTab() fyne.CanvasObject {
	scan := widget.NewButton("Scan tree", func() {
		go func() {
			tree, err := u.client.LibraryTree()
			if err != nil {
				fyne.Do(func() { u.fail(err) })
				return
			}
			fyne.Do(func() {
				u.currentTree = tree
				u.mgmtIdentifyBtn.Enable()
				u.mgmtPreviewBtn.Enable()
				u.mgmtTree.Refresh()
			})
		}()
	})
	u.mgmtIdentifyBtn = widget.NewButton("Identify (TMDB)", func() {
		if u.currentTree == nil {
			return
		}
		go func() {
			out, err := u.client.Identify(*u.currentTree)
			if err != nil {
				fyne.Do(func() { u.fail(err) })
				return
			}
			fyne.Do(func() { u.currentTree = out; u.mgmtTree.Refresh() })
		}()
	})
	u.mgmtPreviewBtn = widget.NewButton("Preview organize", func() {
		if u.currentTree == nil {
			return
		}
		go func() {
			plan, err := u.client.OrganizePreview(*u.currentTree)
			if err != nil {
				fyne.Do(func() { u.fail(err) })
				return
			}
			fyne.Do(func() { u.showOrganizeDialog(plan) })
		}()
	})
	u.mgmtIdentifyBtn.Disable()
	u.mgmtPreviewBtn.Disable()

	buttons := container.NewGridWithColumns(3, scan, u.mgmtIdentifyBtn, u.mgmtPreviewBtn)

	u.mgmtTree = widget.NewTree(
		u.treeChildUIDs,
		u.treeIsBranch,
		u.treeCreate,
		u.treeUpdate,
	)

	u.seasonBarLabel = widget.NewLabel("")
	cancel := widget.NewButton("Cancel", func() {
		go func() {
			if err := u.client.SeasonCancel(); err != nil {
				fyne.Do(func() { u.fail(err) })
			}
		}()
	})
	u.seasonBarWrapper = container.NewBorder(nil, nil, nil, cancel, u.seasonBarLabel)
	u.seasonBarWrapper.Hide()

	bottom := container.NewVBox(u.seasonBarWrapper)
	return container.NewBorder(buttons, bottom, nil, nil, u.mgmtTree)
}

func (u *ui) treeChildUIDs(uid widget.TreeNodeID) []widget.TreeNodeID {
	if u.currentTree == nil {
		return nil
	}
	if uid == "" {
		ids := make([]string, 0, len(u.currentTree.Shows))
		for i := range u.currentTree.Shows {
			ids = append(ids, fmt.Sprintf("s:%d", i))
		}
		return ids
	}
	parts := strings.Split(uid, ":")
	switch parts[0] {
	case "s":
		si, _ := strconv.Atoi(parts[1])
		if si < 0 || si >= len(u.currentTree.Shows) {
			return nil
		}
		ids := make([]string, 0, len(u.currentTree.Shows[si].Seasons))
		for j := range u.currentTree.Shows[si].Seasons {
			ids = append(ids, fmt.Sprintf("e:%d:%d", si, j))
		}
		return ids
	case "e":
		si, _ := strconv.Atoi(parts[1])
		sj, _ := strconv.Atoi(parts[2])
		if si < 0 || si >= len(u.currentTree.Shows) || sj < 0 || sj >= len(u.currentTree.Shows[si].Seasons) {
			return nil
		}
		ids := make([]string, 0)
		for k := range u.currentTree.Shows[si].Seasons[sj].Episodes {
			ids = append(ids, fmt.Sprintf("p:%d:%d:%d", si, sj, k))
		}
		return ids
	}
	return nil
}

func (u *ui) treeIsBranch(uid widget.TreeNodeID) bool {
	if uid == "" {
		return true
	}
	return strings.HasPrefix(uid, "s:") || strings.HasPrefix(uid, "e:")
}

func (u *ui) treeCreate(branch bool) fyne.CanvasObject {
	if branch {
		return container.NewHBox(widget.NewLabel(""), widget.NewButton("Translate…", nil))
	}
	return widget.NewLabel("")
}

func (u *ui) treeUpdate(uid widget.TreeNodeID, branch bool, o fyne.CanvasObject) {
	if u.currentTree == nil {
		return
	}
	if !branch {
		parts := strings.Split(uid, ":")
		si, _ := strconv.Atoi(parts[1])
		sj, _ := strconv.Atoi(parts[2])
		k, _ := strconv.Atoi(parts[3])
		if si >= len(u.currentTree.Shows) || sj >= len(u.currentTree.Shows[si].Seasons) || k >= len(u.currentTree.Shows[si].Seasons[sj].Episodes) {
			return
		}
		ep := u.currentTree.Shows[si].Seasons[sj].Episodes[k]
		name := ep.Name
		if ep.EpisodeName != "" {
			name = fmt.Sprintf("S%02dE%02d %s", ep.Season, ep.Episode, ep.EpisodeName)
		}
		o.(*widget.Label).SetText(name)
		return
	}
	box := o.(*fyne.Container)
	lbl := box.Objects[0].(*widget.Label)
	btn := box.Objects[1].(*widget.Button)
	parts := strings.Split(uid, ":")
	if parts[0] == "s" {
		si, _ := strconv.Atoi(parts[1])
		if si >= len(u.currentTree.Shows) {
			return
		}
		lbl.SetText(u.currentTree.Shows[si].Name)
		btn.Hide()
		return
	}
	si, _ := strconv.Atoi(parts[1])
	sj, _ := strconv.Atoi(parts[2])
	if si >= len(u.currentTree.Shows) || sj >= len(u.currentTree.Shows[si].Seasons) {
		return
	}
	show := u.currentTree.Shows[si]
	season := show.Seasons[sj]
	lbl.SetText(season.Name)
	btn.OnTapped = func() { u.promptTranslateSeason(show.Name, season) }
	btn.Show()
}

func (u *ui) showOrganizeDialog(plan []castapi.OrganizeMove) {
	list := widget.NewList(
		func() int { return len(plan) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*widget.Label).SetText(plan[i].Description)
		},
	)
	scroll := container.NewScroll(list)
	scroll.SetMinSize(fyne.NewSize(360, 400))
	dialog.ShowCustomConfirm("Organize plan", "Execute", "Cancel", scroll, func(ok bool) {
		if !ok {
			return
		}
		go func() {
			if err := u.client.OrganizeExecute(plan); err != nil {
				fyne.Do(func() { u.fail(err) })
			}
		}()
	}, u.window)
}

func (u *ui) promptTranslateSeason(showName string, season castapi.LibrarySeason) {
	langEntry := widget.NewEntry()
	langEntry.SetText("Turkish")
	dialog.ShowCustomConfirm("Translate "+season.Name, "Start", "Cancel",
		container.NewVBox(widget.NewLabel("Language"), langEntry),
		func(ok bool) {
			if !ok {
				return
			}
			lang := strings.TrimSpace(langEntry.Text)
			if lang == "" {
				lang = "Turkish"
			}
			paths := make([]string, 0, len(season.Episodes))
			for _, e := range season.Episodes {
				paths = append(paths, e.Path)
			}
			go func() {
				if err := u.client.TranslateSeason(showName, season.Name, paths, lang); err != nil {
					fyne.Do(func() { u.fail(err) })
					return
				}
				fyne.Do(func() { u.startSeasonPoll() })
			}()
		}, u.window)
}

func (u *ui) startSeasonPoll() {
	u.stopSeasonPoll()
	u.seasonStop = make(chan struct{})
	stop := u.seasonStop
	go func() {
		t := time.NewTicker(2 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-stop:
				return
			case <-t.C:
				st, err := u.client.SeasonStatus()
				if err != nil {
					continue
				}
				running := st.Status == "running"
				text := fmt.Sprintf("Translating %s · %d/%d · %s", st.SeasonName, st.CurrentEpisode, st.TotalEpisodes, st.Message)
				fyne.Do(func() {
					if running {
						u.seasonBarLabel.SetText(text)
						u.seasonBarWrapper.Show()
					} else {
						u.seasonBarWrapper.Hide()
					}
				})
				if !running {
					return
				}
			}
		}
	}()
}

func (u *ui) stopSeasonPoll() {
	if u.seasonStop == nil {
		return
	}
	close(u.seasonStop)
	u.seasonStop = nil
}
