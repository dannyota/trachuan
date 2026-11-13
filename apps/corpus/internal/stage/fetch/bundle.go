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
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
	"golang.org/x/sync/errgroup"
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
	return s.writeBundleObservations(ctx, dir, id, detail, tree, diagram, entries, time.Now, nil)
}

func (s *Stage) writeBundleObservations(ctx context.Context, dir, id string, detail, tree, diagram json.RawMessage, entries []vbpl.File, now func() time.Time, observations *[]stage.FetchObservation) error {
	return s.writeBundleWithBaseline(ctx, dir, id, detail, tree, diagram, entries, now, observations, nil, nil, nil)
}

func (s *Stage) writeBundleWithBaseline(ctx context.Context, dir, id string, detail, tree, diagram json.RawMessage, entries []vbpl.File, now func() time.Time, observations *[]stage.FetchObservation, base *baseline, countsOut *readDecision, q hashStore) error {
	filesDir := filepath.Join(dir, "files")
	if err := os.Mkdir(filesDir, 0700); err != nil {
		return fmt.Errorf("create files directory: %w", err)
	}
	files := make([]file, 0, len(entries))
	result := report{Missing: []string{}, TooLarge: []string{}}
	results := make([]fileResult, len(entries))
	counts := make(map[string]int, len(entries))
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("count attachments: %w", err)
		}
		counts[entry.Name]++
	}
	group, requestCtx := errgroup.WithContext(ctx)
	for i, entry := range entries {
		if err := ctx.Err(); err != nil {
			_ = group.Wait()
			return fmt.Errorf("fetch attachments: %w", err)
		}
		group.Go(func() error {
			d, origin, reason, e := s.attachment(requestCtx, id, filesDir, entry, base, counts[entry.Name] == 1, q)
			results[i] = fileResult{downloaded: d, adopted: origin, reason: reason, err: e, completed: now().UTC().Truncate(time.Microsecond)}
			return nil
		})
	}
	_ = group.Wait()
	conflicts := conflictingAttachments(entries, results)
	recordedConflicts := map[string]bool{}
	hits := 0
	seen := make(map[string]stage.FetchObservation)
	for i, entry := range entries {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("collect attachments: %w", err)
		}
		item := results[i]
		if strings.HasSuffix(item.reason, "_cache") || strings.HasSuffix(item.reason, "_baseline") {
			hits++
		}
		if countsOut != nil && item.reason != "" {
			countsOut.count(item.reason)
		}
		if conflicts[entry.Name] {
			if errors.Is(item.err, errInvalidBaseline) {
				return item.err
			}
			if item.err != nil && !missingFailure(item.err) && !errors.Is(item.err, vbpl.ErrTooLarge) {
				return stage.Retryable(fmt.Errorf("download attachment %q: %w", entry.Name, item.err))
			}
			if !recordedConflicts[entry.Name] {
				recordedConflicts[entry.Name] = true
				result.Missing = append(result.Missing, entry.Name)
				if countsOut != nil {
					countsOut.count("failure_conflict")
				}
				slog.Warn("attachment missing", "document_key", "vn:vbpl:"+id, "file_name", entry.Name, "cause", "conflict")
			}
			continue
		}
		if item.err != nil {
			if errors.Is(item.err, errInvalidBaseline) {
				return item.err
			}
			if ctx.Err() != nil {
				return fmt.Errorf("download attachment: %w", ctx.Err())
			}
			if errors.Is(item.err, vbpl.ErrTooLarge) {
				result.TooLarge = append(result.TooLarge, entry.Name)
			} else if missingFailure(item.err) {
				cause := failureCause(item.err)
				if item.reason == "no_url" {
					cause = "no_url"
				} else if countsOut != nil {
					countsOut.count("failure_" + cause)
				}
				slog.Warn("attachment missing", "document_key", "vn:vbpl:"+id, "file_name", entry.Name, "cause", cause)
				result.Missing = append(result.Missing, entry.Name)
			} else {
				return stage.Retryable(fmt.Errorf("download attachment %q: %w", entry.Name, item.err))
			}
			continue
		}
		files = append(files, file{Name: entry.Name, SHA256: item.downloaded.SHA256, Size: item.downloaded.Size})
		{
			name := entry.Name
			var resourceURL *string
			if entry.URL != "" {
				resourceURL = &entry.URL
			}
			o := stage.FetchObservation{ResourceKey: attachmentKey(entry), Source: "vbpl", Name: &name, URL: resourceURL, Path: "files/" + item.downloaded.SHA256, ContentSHA256: item.downloaded.SHA256, Size: item.downloaded.Size, Acquisition: "network", FetchedAt: &item.completed}
			if item.adopted != nil {
				o.Acquisition = "adopted"
				o.FetchedAt = item.adopted.FetchedAt
				o.OriginRef = item.adopted.OriginRef
			} else if strings.HasSuffix(item.reason, "_cache") {
				o.Acquisition = "cache"
				o.FetchedAt = nil
			}
			if _, ok := seen[o.ResourceKey]; !ok {
				seen[o.ResourceKey] = o
				if observations != nil {
					*observations = append(*observations, o)
				}
			}
		}
	}
	if err := removeConflictingBytes(entries, results, files, conflicts); err != nil {
		return err
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

func (*Stage) Resolve(_ context.Context, env stage.Env, r stage.Run, item stage.Item, art artifact.Result) (stage.Resolution, error) {
	dir := env.Artifacts.Abs(art.Path)
	if _, err := os.Stat(filepath.Join(dir, "document.json")); err == nil {
		return resolveChinhphu(dir, item)
	} else if !os.IsNotExist(err) {
		return stage.Resolution{}, fmt.Errorf("stat document: %w", err)
	}
	detailData, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "detail.json"))
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("read detail: %w", err)
	}
	var detail detailEnvelope
	if err = json.Unmarshal(detailData, &detail); err != nil {
		return stage.Resolution{}, fmt.Errorf("decode detail: %w", err)
	}
	if "vn:vbpl:"+detail.Data.ID != item.Key {
		return stage.Resolution{}, fmt.Errorf("fetch identity disagrees with planned key %q", item.Key)
	}
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
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("parse pin sources: %w", err)
	}
	if pin, ok := list.Pin(item.Key); ok {
		data, err := json.Marshal(pin)
		if err != nil {
			return stage.Resolution{}, fmt.Errorf("encode pin gap: %w", err)
		}
		gaps = append(gaps, stage.Gap{Kind: "source_withdrawn_retained", Detail: string(data)})
	}
	return stage.Resolution{Gaps: gaps}, nil
}
