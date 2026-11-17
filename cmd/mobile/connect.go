package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"

	"wails-cast/pkg/castapi"
)

// ---------------------------------------------------------------------------
// Connect screen
// ---------------------------------------------------------------------------

func (u *ui) showConnect() {
	prefs := u.fyneApp.Preferences()

	host := widget.NewEntry()
	host.SetPlaceHolder("host")
	port := widget.NewEntry()
	port.SetPlaceHolder("port")
	token := widget.NewPasswordEntry()
	token.SetPlaceHolder("token (optional)")

	if last := prefs.String(prefBaseURL); last != "" {
		host.SetText(last)
	}
	if lastTok := prefs.String(prefToken); lastTok != "" {
		token.SetText(lastTok)
	}

	var instances []castapi.CastInstance
	list := widget.NewList(
		func() int { return len(instances) },
		func() fyne.CanvasObject { return widget.NewLabel("") },
		func(i widget.ListItemID, o fyne.CanvasObject) {
			inst := instances[i]
			o.(*widget.Label).SetText(fmt.Sprintf("%s  (%s:%d)", inst.Name, inst.Host, inst.Port))
		},
	)
	list.OnSelected = func(i widget.ListItemID) {
		list.Unselect(i)
		if i < 0 || i >= len(instances) {
			return
		}
		inst := instances[i]
		prefs.SetString(prefBaseURL, inst.URL)
		prefs.SetString(prefToken, token.Text)
		u.client = castapi.New(inst.URL, token.Text)
		u.showMain()
	}

	discover := widget.NewButton("Discover", func() {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			found, err := castapi.Discover(ctx)
			if err != nil {
				fyne.Do(func() { u.fail(err) })
				return
			}
			fyne.Do(func() { instances = found; list.Refresh() })
		}()
	})

	connect := widget.NewButton("Connect", func() {
		p, err := strconv.Atoi(port.Text)
		if err != nil && port.Text != "" {
			u.fail(fmt.Errorf("invalid port: %v", err))
			return
		}
		base := host.Text
		if base == "" {
			u.fail(fmt.Errorf("host is required"))
			return
		}
		if p > 0 {
			base = fmt.Sprintf("http://%s:%d", host.Text, p)
		}
		prefs.SetString(prefBaseURL, base)
		prefs.SetString(prefToken, token.Text)
		u.client = castapi.New(base, token.Text)
		u.showMain()
	})

	form := container.NewVBox(
		widget.NewLabel("Manual connect"),
		host, port, token, connect,
	)
	top := container.NewVBox(
		widget.NewLabel("Discover on LAN"),
		discover,
	)
	u.window.SetContent(container.NewBorder(top, form, nil, nil, list))
}
