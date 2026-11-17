// Command mobile is the phone remote for a frog-cast instance on the LAN.
package main

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/dialog"
)

const appID = "io.frogcast.remote"

func main() {
	a := app.NewWithID(appID)
	w := a.NewWindow("Frog Cast Remote")
	w.Resize(fyne.NewSize(400, 820))

	u := &ui{fyneApp: a, window: w}
	u.showConnect()
	w.SetOnClosed(u.closeSession)
	a.Lifecycle().SetOnEnteredForeground(u.takeShare)

	w.ShowAndRun()
}

// ui owns the window: the connect screen, or a session with an instance.
type ui struct {
	fyneApp fyne.App
	window  fyne.Window
	session *session
	// pendingShare is a shared link waiting for a connection.
	pendingShare string
}

func (u *ui) fail(err error) { dialog.ShowError(err, u.window) }

func (u *ui) closeSession() {
	if u.session != nil {
		u.session.close()
		u.session = nil
	}
}
