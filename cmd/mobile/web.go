package main

import (
	"context"
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

// extractTimeout leaves time to find and tap play on the page.
const extractTimeout = 5 * time.Minute

// webView opens web pages for casting: the instance extracts a page it has
// a script for; any other page opens here for someone to tap play. Either
// way the page's tracks come back from the instance, and its Cast options
// page picks them.
type webView struct {
	s       *session
	content fyne.CanvasObject
	link    *widget.Entry
	open    *widget.Button
	status  *widget.Label
	// options is the Cast options page last opened.
	options *pageOptions
}

func newWebView(s *session) *webView {
	v := &webView{s: s}

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

	hint := note("Pages cast before are in History.")
	hint.SizeName = theme.SizeNameCaptionText
	v.content = container.NewVBox(container.NewBorder(nil, nil, nil, paste, v.link), v.open, v.status, hint)
	return v
}

// show opens a link on the Web page screen, where its progress shows.
func (v *webView) show(link string) {
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
