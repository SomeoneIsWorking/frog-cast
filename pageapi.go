package main

import (
	"encoding/json"
	"net/http"
	"strconv"

	"frog-cast/pkg/castapi"
	"frog-cast/pkg/remote"
)

// readyWebPage makes sure a web page id can be served without a browser on
// this screen: a page that needs one is the controller's to extract, and is
// answered 409. Anything else passes.
func (h *HTTPServer) readyWebPage(w http.ResponseWriter, id string) bool {
	if !isWebURL(id) {
		return true
	}
	if err := h.app.readyPage(id, 0, 0, false); err != nil {
		writeCastError(w, err)
		return false
	}
	return true
}

// handleExtracted stores a web page a controller extracted in its own
// browser as this instance's extraction of it, and returns the page's tracks.
// The controller then casts it with /play like any other page.
// POST /extracted (castapi.ExtractedPage)
func (h *HTTPServer) handleExtracted(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	var page castapi.ExtractedPage
	if err := json.NewDecoder(r.Body).Decode(&page); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
		return
	}
	if !isWebURL(page.PageURL) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "pageUrl must be an http(s) URL"})
		return
	}
	subtitles := make([]remote.PageSubtitleContent, 0, len(page.Subtitles))
	for _, sub := range page.Subtitles {
		subtitles = append(subtitles, remote.PageSubtitleContent{Label: sub.Label, Content: sub.Content})
	}
	data := remote.ExtractionData{
		URL:         page.PageURL,
		Title:       page.Title,
		ManifestURL: page.ManifestURL,
		Headers:     page.Headers,
		Cookies:     page.Cookies,
	}
	if err := h.app.RemoteManager.Seed(data, page.Manifest, subtitles); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	info, err := h.app.GetTrackDisplayInfo(page.PageURL)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, info)
}

// handleDownload reports or drives the download of one track of a web page
// into this instance's cache, so a cast plays from disk.
// GET  /download?url=&type=video|audio&track=N → castapi.DownloadStatus
// POST /download {url, type, track, start}
func (h *HTTPServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	var req castapi.DownloadRequest
	switch r.Method {
	case http.MethodGet:
		q := r.URL.Query()
		track, err := strconv.Atoi(q.Get("track"))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "track must be a number"})
			return
		}
		req = castapi.DownloadRequest{URL: q.Get("url"), Type: q.Get("type"), Track: track}
	case http.MethodPost:
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
			return
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	if !isWebURL(req.URL) || (req.Type != "video" && req.Type != "audio") {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "url must be a web page and type video or audio"})
		return
	}
	if !h.readyWebPage(w, req.URL) {
		return
	}
	if r.Method == http.MethodPost {
		var err error
		if req.Start {
			err = h.app.StartDownload(req.URL, req.Type, req.Track)
		} else {
			err = h.app.StopDownload(req.URL, req.Type, req.Track)
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
	}
	status, err := h.app.GetDownloadStatus(req.URL, req.Type, req.Track)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, downloadStatus(status))
}

func downloadStatus(s *remote.DownloadStatusQeuryResponse) castapi.DownloadStatus {
	out := castapi.DownloadStatus{Status: s.Status, Total: len(s.Segments)}
	for _, done := range s.Segments {
		if done {
			out.Done++
		}
	}
	return out
}
