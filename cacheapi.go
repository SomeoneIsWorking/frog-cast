package main

import (
	"encoding/json"
	"net/http"

	"frog-cast/pkg/castapi"
)

// handleCache reports this instance's media cache and clears it.
// GET  /cache                       → castapi.CacheStats
// POST /cache {"clear": "transcoded"|"video"|"all"} → castapi.CacheStats after
func (h *HTTPServer) handleCache(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
	case http.MethodPost:
		var req castapi.CacheClearRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid JSON body"})
			return
		}
		var err error
		switch req.Clear {
		case castapi.CacheTranscoded:
			err = h.app.DeleteTranscodedCache()
		case castapi.CacheVideo:
			err = h.app.DeleteAllVideoCache()
		case castapi.CacheAll:
			err = h.app.ClearCache()
		default:
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "clear must be transcoded, video or all"})
			return
		}
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
			return
		}
	default:
		writeJSON(w, http.StatusMethodNotAllowed, errorResponse{Error: "method not allowed"})
		return
	}
	stats, err := h.app.GetCacheStats()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, stats)
}
