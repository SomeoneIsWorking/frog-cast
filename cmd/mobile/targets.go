package main

import (
	"errors"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"

	"frog-cast/pkg/castapi"
)

const prefTarget = "castTarget"

// targetPicker chooses where the instance casts to: one of the TVs it can
// see, or itself. The choice is remembered.
type targetPicker struct {
	s       *session
	devices []castapi.RemoteDevice
	host    string
	// selects are the pickers on screen, filled again when targets load.
	selects map[*widget.Select]bool
}

func newTargetPicker(s *session) *targetPicker {
	return &targetPicker{
		s:       s,
		host:    s.ui.fyneApp.Preferences().StringWithFallback(prefTarget, ""),
		selects: map[*widget.Select]bool{},
	}
}

// load asks the instance for its targets; it takes a few seconds.
func (t *targetPicker) load() {
	go func() {
		devs, err := t.s.client.Devices()
		if err != nil {
			return
		}
		fyne.Do(func() {
			t.devices = devs
			if t.indexOf(t.host) < 0 {
				t.host = t.defaultHost()
			}
			for sel := range t.selects {
				t.fill(sel)
			}
		})
	}()
}

// newSelect is a picker over the targets, set to the current choice and
// filled again when they load; picking from it changes the choice. release
// it when it leaves the screen.
func (t *targetPicker) newSelect() *widget.Select {
	sel := widget.NewSelect(nil, nil)
	sel.PlaceHolder = "Looking for TVs…"
	t.selects[sel] = true
	t.fill(sel)
	sel.OnChanged = func(string) {
		if i := sel.SelectedIndex(); i >= 0 {
			t.host = t.devices[i].Host
			t.s.ui.fyneApp.Preferences().SetString(prefTarget, t.host)
		}
	}
	return sel
}

func (t *targetPicker) release(sel *widget.Select) {
	delete(t.selects, sel)
}

func (t *targetPicker) fill(sel *widget.Select) {
	sel.Options = nil
	for _, d := range t.devices {
		sel.Options = append(sel.Options, t.label(d))
	}
	if i := t.indexOf(t.host); i >= 0 {
		sel.SetSelectedIndex(i)
	}
	sel.Refresh()
}

// selected is the target host to cast to; "local" hosts it on the instance.
// Before the targets load there is only a remembered choice.
func (t *targetPicker) selected() (string, error) {
	switch {
	case t.host != "":
		return t.host, nil
	case t.devices == nil:
		return "", errors.New("still looking for TVs; try again in a moment")
	default:
		return t.defaultHost(), nil
	}
}

// name is a target's display name, given the host the instance reports.
func (t *targetPicker) name(host string) string {
	if i := t.indexOf(host); i >= 0 {
		return t.label(t.devices[i])
	}
	return host
}

func (t *targetPicker) label(d castapi.RemoteDevice) string {
	if d.Host == "local" {
		return t.s.name + " (no TV)"
	}
	return d.Name
}

func (t *targetPicker) indexOf(host string) int {
	for i, d := range t.devices {
		if d.Host == host {
			return i
		}
	}
	return -1
}

// defaultHost is the first TV, else the instance itself.
func (t *targetPicker) defaultHost() string {
	for _, d := range t.devices {
		if d.Host != "local" {
			return d.Host
		}
	}
	return "local"
}
