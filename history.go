package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"

	"wails-cast/pkg/events"
	"wails-cast/pkg/folders"
	"wails-cast/pkg/options"
)

const (
	maxHistoryItems = 50
	historyFileName = "cast_history.json"
)

type HistoryItem struct {
	FileNameOrUrl string               `json:"path"`
	Name          string               `json:"name"`
	Timestamp     string               `json:"timestamp"`
	CastOptions   *options.CastOptions `json:"castOptions"`
	// SubtitleDelaySeconds is this media's subtitle timing offset, applied on
	// every cast of it from this instance.
	SubtitleDelaySeconds float64 `json:"subtitleDelaySeconds,omitempty"`
}

type HistoryStore struct {
	items    []HistoryItem
	filePath string
	mu       sync.RWMutex
}

func NewHistoryStore() *HistoryStore {
	appConfigDir := folders.GetConfig()
	os.MkdirAll(appConfigDir, 0755)

	historyPath := filepath.Join(appConfigDir, historyFileName)

	store := &HistoryStore{
		items:    []HistoryItem{},
		filePath: historyPath,
	}

	store.load()
	return store
}

func (h *HistoryStore) load() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	data, err := os.ReadFile(h.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No history file yet
		}
		return err
	}

	return json.Unmarshal(data, &h.items)
}

func (h *HistoryStore) save() error {
	data, err := json.MarshalIndent(h.items, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(h.filePath, data, 0644)
}

func (h *HistoryStore) Add(fileNameOrUrl string, name string, castOptions *options.CastOptions) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	item := HistoryItem{
		FileNameOrUrl: fileNameOrUrl,
		Name:          name,
		Timestamp:     time.Now().Format(time.RFC3339),
		CastOptions:   castOptions,
	}

	// Remove duplicate if exists, keeping its subtitle offset.
	filtered := []HistoryItem{}
	for _, existing := range h.items {
		if existing.FileNameOrUrl != fileNameOrUrl {
			filtered = append(filtered, existing)
		} else {
			item.SubtitleDelaySeconds = existing.SubtitleDelaySeconds
		}
	}

	// Add to beginning
	h.items = append([]HistoryItem{item}, filtered...)

	// Limit size
	if len(h.items) > maxHistoryItems {
		h.items = h.items[:maxHistoryItems]
	}

	err := h.save()
	if err == nil {
		events.Emit("history:updated", item)
	}
	return err
}

// SubtitleDelay returns the subtitle timing offset kept for the media, or 0.
func (h *HistoryStore) SubtitleDelay(fileNameOrUrl string) float64 {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, item := range h.items {
		if item.FileNameOrUrl == fileNameOrUrl {
			return item.SubtitleDelaySeconds
		}
	}
	return 0
}

// SetSubtitleDelay keeps the subtitle timing offset for media in history.
// Media without a history entry (it is added when cast) is skipped.
func (h *HistoryStore) SetSubtitleDelay(fileNameOrUrl string, delay float64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.items {
		if h.items[i].FileNameOrUrl == fileNameOrUrl {
			h.items[i].SubtitleDelaySeconds = delay
			return h.save()
		}
	}
	return nil
}

func (h *HistoryStore) GetAll() []HistoryItem {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// Return a copy
	items := make([]HistoryItem, len(h.items))
	copy(items, h.items)
	return items
}

func (h *HistoryStore) Remove(path string) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	filtered := []HistoryItem{}
	for _, item := range h.items {
		if item.FileNameOrUrl != path {
			filtered = append(filtered, item)
		}
	}

	h.items = filtered
	return h.save()
}

func (h *HistoryStore) Clear() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.items = []HistoryItem{}
	return h.save()
}
