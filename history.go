package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// HistoryEntry records a wallpaper that has been downloaded and set.
type HistoryEntry struct {
	ID       string `json:"id"`
	ThumbURL string `json:"thumb_url"`
	FullURL  string `json:"full_url"`
}

func historyPath() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "govista")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	return filepath.Join(dir, "history.json"), nil
}

func loadHistory() []HistoryEntry {
	path, err := historyPath()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var entries []HistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil
	}
	return entries
}

// appendHistoryEntry deduplicates by ID and prepends the new entry so the
// most recently set wallpaper always appears first.
func appendHistoryEntry(e HistoryEntry) {
	entries := loadHistory()

	fresh := make([]HistoryEntry, 0, len(entries)+1)
	for _, ex := range entries {
		if ex.ID != e.ID {
			fresh = append(fresh, ex)
		}
	}
	all := append([]HistoryEntry{e}, fresh...)

	path, err := historyPath()
	if err != nil {
		return
	}
	data, err := json.MarshalIndent(all, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, data, 0644)
}
