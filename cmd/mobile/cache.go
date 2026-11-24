package main

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
)

// cacheView shows the instance's media cache and clears parts of it.
type cacheView struct {
	s       *session
	content fyne.CanvasObject
	total   *widget.Label
	parts   *widget.Label
}

func newCacheView(s *session) *cacheView {
	v := &cacheView{s: s}
	v.total = widget.NewLabelWithStyle("Media cache", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	v.parts = note("")
	v.parts.SizeName = theme.SizeNameCaptionText
	v.parts.Wrapping = fyne.TextWrapWord
	clear := func(label, part, what string, importance widget.Importance) *widget.Button {
		b := widget.NewButton(label, func() {
			s.ui.confirm("Clear "+what+"?", "This deletes "+what+" on "+s.name+".", "Clear", widget.DangerImportance,
				func() { v.clear(part) })
		})
		b.Importance = importance
		return b
	}
	buttons := container.NewGridWithColumns(3,
		clear("Transcoded", castapi.CacheTranscoded, "the transcoded segments", widget.MediumImportance),
		clear("All video", castapi.CacheVideo, "every video segment, downloads included", widget.MediumImportance),
		clear("Everything", castapi.CacheAll, "the whole cache, page extractions included", widget.DangerImportance),
	)
	v.content = card(container.NewVBox(v.total, v.parts, buttons))
	return v
}

// load reads the cache sizes.
func (v *cacheView) load() {
	go func() {
		st, err := v.s.client.CacheStats()
		fyne.Do(func() {
			if err != nil {
				v.parts.SetText("Unavailable: " + err.Error())
				return
			}
			v.show(st)
		})
	}()
}

func (v *cacheView) clear(part string) {
	go func() {
		st, err := v.s.client.ClearCache(part)
		fyne.Do(func() {
			if err != nil {
				v.s.ui.fail(err)
				return
			}
			v.show(st)
		})
	}()
}

func (v *cacheView) show(st *castapi.CacheStats) {
	v.total.SetText("Media cache · " + fmtBytes(st.Total))
	v.parts.SetText(fmt.Sprintf("Transcoded %s · downloaded %s · metadata %s",
		fmtBytes(st.Transcoded), fmtBytes(st.RawSegments), fmtBytes(st.Metadata)))
}
