//go:build !android

package main

// sharedText: only Android shares text to the app.
func sharedText() string { return "" }
