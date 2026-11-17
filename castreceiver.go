package main

import (
	"time"

	"github.com/vishen/go-chromecast/application"
	cast_proto "github.com/vishen/go-chromecast/cast/proto"
)

// castReceiver is the part of go-chromecast's Application that App drives.
// It is an interface so tests can stand in a fake receiver for the TV.
type castReceiver interface {
	Start(addr string, port int) error
	Load(url string, opts application.LoadOptions) error
	Update() error
	SeekToTime(value float32) error
	Pause() error
	Unpause() error
	SetVolume(value float32) error
	SetMuted(value bool) error
	Close(stopMedia bool) error
	SendCustom(namespace string, typ string, data any) error
}

// newChromecastReceiver builds the real receiver connection, wired to report
// device messages back to the app.
func newChromecastReceiver(a *App) castReceiver {
	app := application.NewApplication()
	app.AddMessageFunc(func(msg *cast_proto.CastMessage) {
		a.handleChromecastMessage(app, msg)
	})
	app.SetRequestTimeout(30 * time.Second)
	return app
}
