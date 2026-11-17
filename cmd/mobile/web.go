package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
)

const (
	prefWebPages = "webPages"
	maxWebPages  = 15
	// extractTimeout leaves time to find and tap play on the page.
	extractTimeout = 5 * time.Minute
)

// webPage is a page cast before, newest first in the list.
type webPage struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

// webView casts web pages: the instance extracts a page it has a script
// for; any other page opens here for someone to tap play.
type webView struct {
	s       *session
	content fyne.CanvasObject
	pages   []webPage
	list    *widget.List
	status  *widget.Label
}

func newWebView(s *session) *webView {
	v := &webView{s: s}
	_ = json.Unmarshal([]byte(s.ui.fyneApp.Preferences().String(prefWebPages)), &v.pages)

	link := widget.NewEntry()
	link.SetPlaceHolder("Page link")
	cast := widget.NewButtonWithIcon("Cast", theme.MediaPlayIcon(), func() {
		v.castSheet(webPage{URL: link.Text})
	})
	cast.Importance = widget.HighImportance
	paste := widget.NewButtonWithIcon("", theme.ContentPasteIcon(), func() {
		link.SetText(s.ui.fyneApp.Clipboard().Content())
	})
	paste.Importance = widget.LowImportance

	v.list = widget.NewList(
		func() int { return len(v.pages) },
		func() fyne.CanvasObject { return newRow(theme.MediaVideoIcon()) },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			page := v.pages[i]
			title := page.Title
			if title == "" {
				title = page.URL
			}
			host := page.URL
			if u, err := url.Parse(page.URL); err == nil {
				host = u.Host
			}
			setRow(o, title, host)
		},
	)
	v.list.OnSelected = func(i widget.ListItemID) {
		v.list.Unselect(i)
		v.castSheet(v.pages[i])
	}

	v.status = caption("A page the instance has no script for opens here: tap play on it and the stream goes to the TV.")
	top := container.NewVBox(
		container.NewBorder(nil, nil, nil, paste, link),
		cast,
		v.status,
		heading1("Recent"),
	)
	v.content = container.NewBorder(container.NewPadded(top), nil, nil, nil, v.list)
	return v
}

// castSheet picks the target and quality for a page and casts it.
func (v *webView) castSheet(page webPage) {
	if u, err := url.Parse(page.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		v.s.ui.fail(errors.New("enter a web page link (http or https)"))
		return
	}
	target := v.s.targets.newSelect()
	quality := widget.NewSelect(qualityLabels, nil)
	quality.PlaceHolder = "Instance default"
	title := page.Title
	if title == "" {
		title = "Cast page"
	}
	form := dialog.NewForm(title, "Cast", "Cancel", []*widget.FormItem{
		widget.NewFormItem("Cast to", target),
		widget.NewFormItem("Quality", quality),
	}, func(ok bool) {
		if !ok {
			return
		}
		opts := castapi.PlayOptions{SubtitlePath: "none"}
		if i := quality.SelectedIndex(); i >= 0 {
			opts.Quality = &qualityValues[i]
		}
		v.cast(page, v.s.targets.selected(), opts)
	}, v.s.ui.window)
	form.Resize(fyne.NewSize(v.s.ui.window.Canvas().Size().Width-16, form.MinSize().Height))
	form.Show()
}

func (v *webView) cast(page webPage, host string, opts castapi.PlayOptions) {
	v.status.SetText("Asking the instance…")
	go func() {
		st, err := v.s.client.Play(page.URL, host, opts)
		if errors.Is(err, castapi.ErrNeedsExtraction) {
			fyne.Do(func() { v.status.SetText("Tap play on the page; it closes once the stream is found.") })
			st, err = v.extractHere(page.URL, host, opts)
		}
		fyne.Do(func() {
			v.status.SetText("")
			if err != nil {
				v.s.ui.fail(err)
				return
			}
			page.Title = st.MediaName
			v.remember(page)
			v.s.showPlaying()
		})
	}()
}

// extractHere finds the page's stream on this phone and has the instance
// cast it.
func (v *webView) extractHere(pageURL, host string, opts castapi.PlayOptions) (*castapi.PlaybackState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), extractTimeout)
	defer cancel()
	res, err := extractPage(ctx, pageURL)
	if err != nil {
		return nil, err
	}
	page := castapi.ExtractedPage{
		PageURL:     pageURL,
		Title:       res.Title,
		ManifestURL: res.ManifestURL,
		Manifest:    res.Manifest,
		Headers:     res.Headers,
		Cookies:     res.Cookies,
	}
	for _, sub := range res.Subtitles {
		page.Subtitles = append(page.Subtitles, castapi.PageSubtitle{Label: sub.Label, Content: sub.Content})
	}
	return v.s.client.PlayExtracted(castapi.PlayExtractedRequest{Page: page, DeviceIP: host, PlayOptions: opts})
}

func (v *webView) remember(page webPage) {
	pages := []webPage{page}
	for _, p := range v.pages {
		if p.URL != page.URL && len(pages) < maxWebPages {
			pages = append(pages, p)
		}
	}
	v.pages = pages
	if data, err := json.Marshal(pages); err == nil {
		v.s.ui.fyneApp.Preferences().SetString(prefWebPages, string(data))
	}
	v.list.Refresh()
}

