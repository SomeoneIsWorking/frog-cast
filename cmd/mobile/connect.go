package main

import (
	"context"
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
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
	list := cardList(
		func() int { return len(instances) },
		func() fyne.CanvasObject { return newRow(theme.ComputerIcon()) },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			o.(*row).set(instances[i].Name, fmt.Sprintf("%s:%d", instances[i].Host, instances[i].Port))
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

	heading := section("ON THIS NETWORK")
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

	logo, _ := badge(theme.MediaVideoIcon())
	hero := container.NewBorder(nil, nil, container.NewCenter(logo), nil, container.New(layout.NewCustomPaddedVBoxLayout(-16),
		heading1("Frog Cast"), note("Remote for the casts on your network")))
	top := []fyne.CanvasObject{container.New(layout.NewCustomPaddedLayout(12, 12, 0, 0), hero)}
	if last != "" {
		again := widget.NewButtonWithIcon("Reconnect to "+lastName, theme.MediaPlayIcon(), func() { connect(lastName, last) })
		again.Importance = widget.HighImportance
		top = append(top, again)
	}

	u.window.SetContent(container.New(layout.NewCustomPaddedLayout(12, 12, 14, 14), container.NewBorder(
		container.NewVBox(top...), card(more), nil, nil, found)))
	search()
}
