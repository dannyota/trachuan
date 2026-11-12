package fetch

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

type fileResult struct {
	downloaded vbpl.Downloaded
	adopted    *stage.FetchObservation
	reason     string
	completed  time.Time
	err        error
}

func attachmentKey(entry vbpl.File) string {
	var resourceURL *string
	if entry.URL != "" {
		resourceURL = &entry.URL
	}
	key, _ := json.Marshal([3]any{"attachment", entry.Name, resourceURL})
	return string(key)
}

func conflictingAttachments(entries []vbpl.File, results []fileResult) map[string]bool {
	seen := map[string]vbpl.Downloaded{}
	conflicts := map[string]bool{}
	for i, entry := range entries {
		item := results[i]
		if item.err != nil {
			continue
		}
		key := attachmentKey(entry)
		prior, ok := seen[key]
		if ok && (prior.SHA256 != item.downloaded.SHA256 || prior.Size != item.downloaded.Size) {
			conflicts[entry.Name] = true
		}
		seen[key] = item.downloaded
	}
	return conflicts
}

func removeConflictingBytes(entries []vbpl.File, results []fileResult, files []file, conflicts map[string]bool) error {
	keep := map[string]bool{}
	for _, f := range files {
		keep[f.SHA256] = true
	}
	for i, entry := range entries {
		d := results[i].downloaded
		if !conflicts[entry.Name] || d.Path == "" || keep[d.SHA256] {
			continue
		}
		if err := os.Remove(d.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove conflicting attachment: %w", err)
		}
	}
	return nil
}
