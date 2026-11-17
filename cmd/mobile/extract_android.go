//go:build android

package main

import (
	"context"

	"fyne.io/fyne/v2/driver"

	"wails-cast/pkg/webextract"
)

// extractPage shows the page over the app until it plays a stream.
func extractPage(ctx context.Context, pageURL string) (*webextract.Result, error) {
	host := webextract.Android{Run: func(fn func(vm, env, activity uintptr) error) error {
		return driver.RunNative(func(c any) error {
			ac := c.(*driver.AndroidContext)
			return fn(ac.VM, ac.Env, ac.Ctx)
		})
	}}
	return host.Extract(ctx, pageURL, webextract.Rules{}, webextract.Options{Visible: true})
}
