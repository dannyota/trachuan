package fetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"

	"danny.vn/traquydinh/apps/corpus/internal/artifact"
	"danny.vn/traquydinh/apps/corpus/internal/stage"
	"danny.vn/traquydinh/apps/corpus/internal/vbpl"
)

type file struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type report struct {
	Missing  []string `json:"missing"`
	TooLarge []string `json:"too_large"`
}

func (s *Stage) writeBundle(ctx context.Context, dir, id string, detail, tree, diagram json.RawMessage, entries []vbpl.File) error {
	filesDir := filepath.Join(dir, "files")
	if err := os.Mkdir(filesDir, 0700); err != nil {
		return fmt.Errorf("create files directory: %w", err)
	}
	files := make([]file, 0, len(entries))
	result := report{Missing: []string{}, TooLarge: []string{}}
	hits := 0
	for _, entry := range entries {
		downloaded, hit := s.cachedFile(id, filesDir, entry)
		var err error
		if hit {
			hits++
		} else {
			downloaded, err = s.client.Download(ctx, id, entry, filesDir, s.fileLimit)
		}
		if err != nil {
			if ctx.Err() != nil {
				return fmt.Errorf("download attachment: %w", ctx.Err())
			}
			if errors.Is(err, vbpl.ErrTooLarge) {
				result.TooLarge = append(result.TooLarge, entry.Name)
			} else {
				result.Missing = append(result.Missing, entry.Name)
			}
			continue
		}
		files = append(files, file{Name: entry.Name, SHA256: downloaded.SHA256, Size: downloaded.Size})
	}
	if hits > 0 {
		slog.Info("attachment cache hits", "document_key", "vn:vbpl:"+id, "count", hits)
	}
	sort.Slice(files, func(i, j int) bool {
		if files[i].Name != files[j].Name {
			return files[i].Name < files[j].Name
		}
		return files[i].SHA256 < files[j].SHA256
	})
	sort.Strings(result.Missing)
	sort.Strings(result.TooLarge)
	for _, entry := range []struct {
		name  string
		value any
	}{{"detail.json", detail}, {"tree.json", tree}, {"diagram.json", diagram}, {"files.json", files}, {"report.json", result}} {
		var data []byte
		var err error
		if raw, ok := entry.value.(json.RawMessage); ok {
			data = raw
		} else {
			data, err = json.Marshal(entry.value)
			if err != nil {
				return fmt.Errorf("encode %s: %w", entry.name, err)
			}
		}
		if err = os.WriteFile(filepath.Join(dir, entry.name), data, 0600); err != nil {
			return fmt.Errorf("write %s: %w", entry.name, err)
		}
	}
	return nil
}

// Resolve returns gaps recorded in the bundle for this run.
func (*Stage) Resolve(_ context.Context, env stage.Env, _ stage.Run, _ stage.Item, art artifact.Result) (stage.Resolution, error) {
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "report.json"))
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("read fetch report: %w", err)
	}
	var result report
	if err = json.Unmarshal(data, &result); err != nil {
		return stage.Resolution{}, fmt.Errorf("decode fetch report: %w", err)
	}
	var gaps []stage.Gap
	for _, name := range result.Missing {
		gaps = append(gaps, stage.Gap{Kind: "missing_attachment", Detail: name})
	}
	for _, name := range result.TooLarge {
		gaps = append(gaps, stage.Gap{Kind: "file_too_large", Detail: name})
	}
	return stage.Resolution{Gaps: gaps}, nil
}
