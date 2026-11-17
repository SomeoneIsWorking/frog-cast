package main

import "regexp"

var linkPattern = regexp.MustCompile(`https?://\S+`)

// sharedLink is the first web link in shared text; browsers share the page
// title and its URL.
func sharedLink(text string) string {
	return linkPattern.FindString(text)
}

// takeShare opens the cast sheet for a link shared to the app, now or once
// connected.
func (u *ui) takeShare() {
	if link := sharedLink(sharedText()); link != "" {
		u.pendingShare = link
	}
	if u.pendingShare != "" && u.session != nil {
		link := u.pendingShare
		u.pendingShare = ""
		u.session.web.share(link)
	}
}
