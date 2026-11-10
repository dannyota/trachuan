package build

import (
	"sort"
	"time"
)

type provenanceFile struct {
	DocumentKey string     `json:"document_key"`
	Source      string     `json:"source"`
	Name        string     `json:"name"`
	SHA256      string     `json:"sha256"`
	Size        int64      `json:"size"`
	Path        string     `json:"path"`
	URL         *string    `json:"url"`
	Role        *string    `json:"role"`
	FetchedAt   *time.Time `json:"fetched_at"`
}
type provenance struct {
	Version int              `json:"version"`
	Files   []provenanceFile `json:"files"`
}

func provenanceSnapshot(files []provenanceFile, present map[string]bool) provenance {
	result := provenance{Version: 1, Files: []provenanceFile{}}
	for _, f := range files {
		if present[f.DocumentKey] {
			if f.FetchedAt != nil {
				utc := f.FetchedAt.UTC()
				f.FetchedAt = &utc
			}
			result.Files = append(result.Files, f)
		}
	}
	sort.Slice(result.Files, func(a, b int) bool {
		x, y := result.Files[a], result.Files[b]
		if x.DocumentKey != y.DocumentKey {
			return x.DocumentKey < y.DocumentKey
		}
		if x.Name != y.Name {
			return x.Name < y.Name
		}
		return x.SHA256 < y.SHA256
	})
	return result
}
