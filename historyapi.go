package main

import (
	"net/http"

	"frog-cast/pkg/castapi"
)

// handleHistory lists what this instance has cast, newest first, and
// removes entries.
// GET    /history            → castapi.HistoryResponse
// DELETE /history?path=<id>  removes one entry; without path, all of them
func (h *HTTPServer) handleHistory(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items := []castapi.HistoryItem{}
		for _, it := range h.app.GetHistory() {
			items = append(items, castapi.HistoryItem{Path: it.FileNameOrUrl, Name: it.Name, Timestamp: it.Timestamp, CastOptions: it.CastOptions})
		}
		writeJSON(w, http.StatusOK, castapi.HistoryResponse{Items: items})
	case http.MethodDelete:
		var err error
		if path := r.URL.Query().Get("path"); path != "" {
			err = h.app.RemoveFromHistory(path)
		} else {
			err = h.app.ClearHistory()
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, castapi.OKResponse{OK: true})
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
	}
}
