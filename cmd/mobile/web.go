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

// webView opens web pages for casting: the instance extracts a page it has
// a script for; any other page opens here for someone to tap play. Either
// way the page's tracks come back from the instance, and its Cast options
// page picks them.
type webView struct {
	s       *session
	content fyne.CanvasObject
	pages   []webPage
	list    *widget.List
	recent  *widget.Label
	link    *widget.Entry
	open    *widget.Button
	status  *widget.Label
	// options is the Cast options page last opened.
	options *pageOptions
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

	v.open = widget.NewButtonWithIcon("Open", theme.SearchIcon(), func() { v.openPage(v.link.Text) })
	v.open.Importance = widget.HighImportance
	v.status = caption("")
	v.status.Hide()

	v.recent = section("RECENT PAGES")
	v.list = cardList(
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
			o.(*row).set(title, host)
		},
	)
	v.list.OnSelected = func(i widget.ListItemID) {
		v.list.Unselect(i)
		v.link.SetText(v.pages[i].URL)
		v.openPage(v.pages[i].URL)
	}
	v.recent.Hidden = len(v.pages) == 0

	top := container.NewVBox(container.NewBorder(nil, nil, nil, paste, v.link), v.open, v.status, v.recent)
	v.content = container.NewBorder(top, nil, nil, nil, v.list)
	return v
}

// share shows a link shared to the app and opens it.
func (v *webView) share(link string) {
	v.s.tabs.Select(tabLibrary)
	if v.s.library.pages.top() != v.content {
		v.s.library.pages.push("Web page", v.content)
	}
	v.link.SetText(link)
	v.openPage(link)
}

func (v *webView) setStatus(text string) {
	v.status.SetText(text)
	v.status.Hidden = text == ""
	v.status.Refresh()
}

// openPage finds the page's stream and shows its Cast options.
func (v *webView) openPage(link string) {
	pageURL := strings.TrimSpace(link)
	if u, err := url.Parse(pageURL); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		v.s.ui.fail(errors.New("enter a web page link (http or https)"))
		return
	}
	v.open.Disable()
	v.setStatus(v.s.name + " is finding the page's stream…")
	go func() {
		info, err := v.s.client.TrackInfo(pageURL)
		if errors.Is(err, castapi.ErrNeedsExtraction) {
			fyne.Do(func() { v.setStatus("Tap play on the page; it closes once the stream is found.") })
			info, err = v.extractHere(pageURL)
		}
		fyne.Do(func() {
			v.open.Enable()
			v.setStatus("")
			if err != nil {
				v.s.ui.fail(err)
				return
			}
			if v.options != nil {
				v.options.close()
			}
			v.options = newPageOptions(v, pageURL, info)
			v.s.library.pages.push("Cast options", v.options.content)
		})
	}()
}

// extractHere finds the page's stream on this phone and gives it to the
// instance, which returns the page's tracks.
func (v *webView) extractHere(pageURL string) (*castapi.TrackDisplayInfo, error) {
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
	return v.s.client.SendExtracted(page)
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
