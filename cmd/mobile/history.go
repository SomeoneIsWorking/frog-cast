package main

import (
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
)

// historyView lists what the instance has cast, newest first: tapping an
// entry opens it again (a file's play sheet, a page's Cast options).
type historyView struct {
	s       *session
	content fyne.CanvasObject
	items   []castapi.HistoryItem
	list    *widget.List
	empty   fyne.CanvasObject
}

func newHistoryView(s *session) *historyView {
	v := &historyView{s: s}
	v.list = cardList(
		func() int { return len(v.items) },
		func() fyne.CanvasObject {
			remove := widget.NewButtonWithIcon("", theme.CancelIcon(), nil)
			remove.Importance = widget.LowImportance
			return newRowWith(theme.MediaVideoIcon(), remove)
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			it := v.items[i]
			r := o.(*row)
			icon := theme.FileVideoIcon()
			if isWebPage(it.Path) {
				icon = theme.MediaVideoIcon()
			}
			r.icon.SetResource(theme.NewPrimaryThemedResource(icon))
			r.set(it.Name, historyDetail(it, time.Now()))
			r.trail.(*widget.Button).OnTapped = func() { v.remove(it.Path) }
		},
	)
	v.list.OnSelected = func(i widget.ListItemID) {
		v.list.Unselect(i)
		it := v.items[i]
		if isWebPage(it.Path) {
			s.web.show(it.Path)
		} else {
			s.showPlaySheet(it.Path, it.Name)
		}
	}
	v.empty = emptyState(theme.HistoryIcon(), "Nothing cast yet", "What "+s.name+" casts shows up here.")
	clearAll := widget.NewButtonWithIcon("Clear history", theme.DeleteIcon(), func() {
		s.ui.confirm("Clear history?", "This clears what "+s.name+" has cast.", "Clear", widget.DangerImportance,
			func() { v.remove("") })
	})
	clearAll.Importance = widget.LowImportance
	v.content = container.NewBorder(nil, clearAll, nil, nil, container.NewStack(v.list, v.empty))
	return v
}

// load reads the instance's history.
func (v *historyView) load() {
	go func() {
		items, err := v.s.client.History()
		fyne.Do(func() {
			if err != nil {
				v.s.ui.fail(err)
				return
			}
			v.show(items)
		})
	}()
}

// remove drops one entry, or every entry for "".
func (v *historyView) remove(path string) {
	go func() {
		err := v.s.client.RemoveHistory(path)
		fyne.Do(func() {
			if err != nil {
				v.s.ui.fail(err)
				return
			}
			v.load()
		})
	}()
}

func (v *historyView) show(items []castapi.HistoryItem) {
	v.items = items
	if len(items) > 0 {
		v.empty.Hide()
	} else {
		v.empty.Show()
	}
	v.list.Refresh()
}

func isWebPage(path string) bool {
	return strings.HasPrefix(path, "http://") || strings.HasPrefix(path, "https://")
}

// historyDetail says when and where an entry is from: "2 h ago · site.com".
func historyDetail(it castapi.HistoryItem, now time.Time) string {
	where := filepath.Base(filepath.Dir(it.Path))
	if u, err := url.Parse(it.Path); err == nil && isWebPage(it.Path) {
		where = strings.TrimPrefix(u.Host, "www.")
	}
	t, err := time.Parse(time.RFC3339, it.Timestamp)
	if err != nil {
		return where
	}
	return ago(now.Sub(t)) + " · " + where
}

// ago is a rough age: "just now", "5 min ago", "3 h ago", "2 days ago".
func ago(d time.Duration) string {
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	case d < 48*time.Hour:
		return "yesterday"
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
