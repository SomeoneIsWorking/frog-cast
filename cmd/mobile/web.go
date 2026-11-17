package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
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
	recent  *widget.Label
	link    *widget.Entry
	quality *widget.Select
	cast    *widget.Button
	status  *widget.Label
}

func newWebView(s *session) *webView {
	v := &webView{s: s}
	_ = json.Unmarshal([]byte(s.ui.fyneApp.Preferences().String(prefWebPages)), &v.pages)

	// Wrapped, so a whole link shows without scrolling sideways.
	v.link = widget.NewMultiLineEntry()
	v.link.Wrapping = fyne.TextWrapBreak
	v.link.SetMinRowsVisible(2)
	v.link.SetPlaceHolder("Paste or share a page link")
	paste := widget.NewButtonWithIcon("", theme.ContentPasteIcon(), func() {
		v.link.SetText(sharedLink(s.ui.fyneApp.Clipboard().Content()))
	})
	paste.Importance = widget.LowImportance

	v.quality = widget.NewSelect(qualityLabels, nil)
	v.quality.PlaceHolder = "Instance default"
	form := widget.NewForm(
		widget.NewFormItem("Cast to", s.targets.newSelect()),
		widget.NewFormItem("Quality", v.quality),
	)

	v.cast = widget.NewButtonWithIcon("Cast", theme.MediaPlayIcon(), v.castLink)
	v.cast.Importance = widget.HighImportance
	v.status = caption("")
	v.status.Hide()

	v.recent = widget.NewLabelWithStyle("Recent pages", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
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
		v.link.SetText(v.pages[i].URL)
	}
	v.recent.Hidden = len(v.pages) == 0

	top := container.NewVBox(container.NewBorder(nil, nil, nil, paste, v.link), form, v.cast, v.status, widget.NewSeparator(), v.recent)
	v.content = container.NewBorder(container.NewPadded(top), nil, nil, nil, v.list)
	return v
}

// share shows a link shared to the app, ready to cast.
func (v *webView) share(link string) {
	v.s.tabs.SelectIndex(0)
	if v.s.library.pages.top() != v.content {
		v.s.library.pages.push("Web page", v.content)
	}
	v.link.SetText(link)
}

func (v *webView) setStatus(text string) {
	v.status.SetText(text)
	v.status.Hidden = text == ""
	v.status.Refresh()
}

// castLink casts the page in the link field to the chosen target.
func (v *webView) castLink() {
	page := webPage{URL: strings.TrimSpace(v.link.Text)}
	if u, err := url.Parse(page.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		v.s.ui.fail(errors.New("enter a web page link (http or https)"))
		return
	}
	host, err := v.s.targets.selected()
	if err != nil {
		v.s.ui.fail(err)
		return
	}
	opts := castapi.PlayOptions{SubtitlePath: "none"}
	if i := v.quality.SelectedIndex(); i >= 0 {
		opts.Quality = &qualityValues[i]
	}
	v.cast.Disable()
	v.setStatus("Asking " + v.s.name + "…")
	go func() {
		st, err := v.s.client.Play(page.URL, host, opts)
		if errors.Is(err, castapi.ErrNeedsExtraction) {
			fyne.Do(func() { v.setStatus("Tap play on the page; it closes once the stream is found.") })
			st, err = v.extractHere(page.URL, host, opts)
		}
		fyne.Do(func() {
			v.cast.Enable()
			v.setStatus("")
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
	v.recent.Show()
	v.list.Refresh()
}
