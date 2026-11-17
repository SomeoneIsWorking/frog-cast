//go:build !android

package main

import (
	"context"
	"errors"

	"frog-cast/pkg/webextract"
)

// extractPage is only on Android; elsewhere cast from the instance's machine.
func extractPage(context.Context, string) (*webextract.Result, error) {
	return nil, errors.New("this page needs a browser: cast it from the phone app or the instance's machine")
}
