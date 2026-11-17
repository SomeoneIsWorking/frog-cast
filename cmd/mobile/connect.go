package main

import (
	"context"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"wails-cast/pkg/castapi"
)

const (
	prefBaseURL = "baseURL"
	prefName    = "instanceName"
	prefToken   = "token"
)

// showConnect lists the instances found on the LAN (searched on open) and
// offers the last one used and a manual address.
func (u *ui) showConnect() {
	u.closeSession()
	prefs := u.fyneApp.Preferences()

	token := widget.NewPasswordEntry()
	token.SetPlaceHolder("Token (if the instance has one)")
	token.SetText(prefs.String(prefToken))

	connect := func(name, base string) {
		prefs.SetString(prefBaseURL, base)
		prefs.SetString(prefName, name)
		prefs.SetString(prefToken, token.Text)
		u.openSession(name, castapi.New(base, token.Text))
	}

	// The last instance used; discovery corrects its address when the
	// instance moved or was first reached through another interface.
	last, lastName := prefs.String(prefBaseURL), prefs.String(prefName)
	if lastName == "" {
		lastName = strings.TrimPrefix(last, "http://")
	}

	var instances []castapi.CastInstance
	list := widget.NewList(
		func() int { return len(instances) },
		func() fyne.CanvasObject {
			name := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
			addr := widget.NewLabel("")
			addr.Importance = widget.LowImportance
			return container.NewBorder(nil, nil, widget.NewIcon(theme.ComputerIcon()), widget.NewIcon(theme.NavigateNextIcon()),
				container.NewVBox(name, addr))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			texts := o.(*fyne.Container).Objects[0].(*fyne.Container).Objects
			texts[0].(*widget.Label).SetText(instances[i].Name)
			texts[1].(*widget.Label).SetText(fmt.Sprintf("%s:%d", instances[i].Host, instances[i].Port))
		},
	)
	list.OnSelected = func(i widget.ListItemID) {
		list.Unselect(i)
		connect(instances[i].Name, instances[i].URL)
	}

	searching := widget.NewActivity()
	status := widget.NewLabel("")
	status.Importance = widget.LowImportance
	status.Alignment = fyne.TextAlignCenter
	var rescan *widget.Button
	search := func() {
		rescan.Disable()
		searching.Start()
		searching.Show()
		status.SetText("Searching the network…")
		go func() {
			found, err := castapi.Discover(context.Background())
			fyne.Do(func() {
				searching.Stop()
				searching.Hide()
				rescan.Enable()
				instances = found
				list.Refresh()
				for _, in := range found {
					if in.Name == lastName && in.URL != last {
						last = in.URL
						prefs.SetString(prefBaseURL, last)
					}
				}
				switch {
				case err != nil:
					status.SetText("Search failed: " + err.Error())
				case len(found) == 0:
					status.SetText("No instances found. Is the remote API on?")
				default:
					status.SetText("")
				}
			})
		}()
	}
	rescan = widget.NewButtonWithIcon("", theme.ViewRefreshIcon(), search)
	rescan.Importance = widget.LowImportance

	heading := widget.NewLabel("On this network")
	heading.TextStyle = fyne.TextStyle{Bold: true}
	found := container.NewBorder(
		container.NewBorder(nil, nil, nil, container.NewHBox(searching, rescan), heading),
		status, nil, nil, list)

	address := widget.NewEntry()
	address.SetPlaceHolder("host:port, e.g. 192.168.1.20:9999")
	if last != "" {
		address.SetText(strings.TrimPrefix(last, "http://"))
	}
	manual := widget.NewButtonWithIcon("Connect", theme.LoginIcon(), func() {
		base := strings.TrimSpace(address.Text)
		if base == "" {
			u.fail(fmt.Errorf("enter the instance's address"))
			return
		}
		if !strings.Contains(base, "://") {
			base = "http://" + base
		}
		connect(strings.TrimPrefix(base, "http://"), base)
	})
	manualForm := container.NewVBox(address, token, manual)
	more := widget.NewAccordion(widget.NewAccordionItem("Connect by address", manualForm))

	top := []fyne.CanvasObject{heading1("Cast Remote"), caption("Control wails-cast on your network")}
	if last != "" {
		again := widget.NewButtonWithIcon("Reconnect to "+lastName, theme.MediaPlayIcon(), func() { connect(lastName, last) })
		again.Importance = widget.HighImportance
		top = append(top, again)
	}

	u.window.SetContent(container.NewPadded(container.NewBorder(
		container.NewVBox(top...), more, nil, nil, found)))
	search()
}

// heading1 is a screen title.
func heading1(text string) *widget.Label {
	l := widget.NewLabelWithStyle(text, fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	l.SizeName = theme.SizeNameHeadingText
	return l
}

// caption is secondary text under a title or row, wrapped to its width.
func caption(text string) *widget.Label {
	l := note(text)
	l.Wrapping = fyne.TextWrapWord
	return l
}

// note is a one-line secondary text, for places with no width to wrap to
// (a centered message).
func note(text string) *widget.Label {
	l := widget.NewLabel(text)
	l.Importance = widget.LowImportance
	return l
}
