package fetch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

// The limit is set in docs/design/budgets.md.
const reuseAgeLimit = 30 * 24 * time.Hour

type readDecision struct {
	reason   string
	baseline *baseline
}

type baseline struct {
	invalid                             bool
	row                                 store.GetFetchBaselineRow
	dir                                 string
	detail, tree, filesJSON, reportJSON []byte
	files                               []file
	report                              report
	observations                        []store.RunFetchObservation
}

func digest(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

func updatedDate(data []byte) json.RawMessage {
	var envelope struct {
		Data map[string]json.RawMessage `json:"data"`
	}
	if json.Unmarshal(data, &envelope) != nil {
		return nil
	}
	value := envelope.Data["updatedDate"]
	var s string
	if len(value) == 0 || value[0] != '"' || json.Unmarshal(value, &s) != nil {
		return nil
	}
	return value
}

func (b *baseline) origin(path, hash string) *store.RunFetchObservation {
	for i := range b.observations {
		o := &b.observations[i]
		if o.Path == path && o.ContentSha256 == hash {
			return o
		}
	}
	return nil
}

func (b *baseline) fileOrigin(f file) *store.RunFetchObservation {
	for i := range b.observations {
		o := &b.observations[i]
		if o.Path != "files/"+f.SHA256 || o.ContentSha256 != f.SHA256 || o.Size != f.Size {
			continue
		}
		a := adopted(o)
		if a.Name != nil && *a.Name == f.Name {
			return o
		}
	}
	return nil
}

func (b *baseline) expired(now time.Time, limit time.Duration) bool {
	tree := b.origin("tree.json", digest(b.tree))
	if tree == nil || tree.FetchedAt == nil || tree.FetchedAt.Before(now.Add(-limit)) {
		return true
	}
	for _, f := range b.files {
		found := false
		for i := range b.observations {
			o := &b.observations[i]
			if o.Path != "files/"+f.SHA256 || o.ContentSha256 != f.SHA256 || o.Size != f.Size {
				continue
			}
			obs := adopted(o)
			if obs.Name == nil || *obs.Name != f.Name {
				continue
			}
			found = true
			if o.FetchedAt == nil || o.FetchedAt.Before(now.Add(-limit)) {
				return true
			}
		}
		if !found {
			return true
		}
	}
	return false
}

func reuseDecision(fresh []byte, b *baseline, now time.Time, limit time.Duration) string {
	if b == nil {
		return "no_baseline"
	}
	if b.invalid {
		return "invalid_baseline"
	}
	a, old := updatedDate(fresh), updatedDate(b.detail)
	if a == nil || old == nil {
		return "null_date"
	}
	if !bytes.Equal(a, old) {
		return "date_changed"
	}
	if len(b.report.Missing) > 0 {
		return "missing_attachment"
	}
	if b.expired(now, limit) {
		return "age"
	}
	return "reused"
}

func loadBaseline(ctx context.Context, env stage.Env, r stage.Run, item stage.Item) (*baseline, bool, error) {
	if env.DB == nil {
		return nil, false, nil
	}
	q := store.New(env.DB)
	recipe := r.Snapshot.Recipes["fetch"]
	row, err := q.GetFetchBaseline(ctx, store.GetFetchBaselineParams{DocumentKey: item.Key, FetchRecipeHash: recipe})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read fetch baseline: %w", err)
	}
	miss := func(reason string) (*baseline, bool, error) {
		slog.Warn("invalid fetch baseline", "run", r.Country+"/"+r.Rel, "key", item.Key, "reason", reason)
		return nil, true, nil
	}
	art, ok, err := env.Artifacts.OpenRecorded("fetch", row.Sha256, recipe, row.Sha256)
	if err != nil {
		return nil, false, err
	}
	if !ok || filepath.Clean(env.Artifacts.Abs(row.Path)) != filepath.Clean(env.Artifacts.Abs(art.Path)) {
		return miss("bundle missing, invalid, or path mismatch")
	}
	b := &baseline{row: row, dir: env.Artifacts.Abs(art.Path)}
	for _, entry := range []struct {
		name string
		dst  *[]byte
	}{{"detail.json", &b.detail}, {"tree.json", &b.tree}, {"files.json", &b.filesJSON}, {"report.json", &b.reportJSON}} {
		*entry.dst, err = os.ReadFile(filepath.Join(b.dir, entry.name))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return miss(err.Error())
			}
			return nil, false, fmt.Errorf("read baseline %s: %w", entry.name, err)
		}
	}
	var detail detailEnvelope
	if json.Unmarshal(b.detail, &detail) != nil || "vn:vbpl:"+detail.Data.ID != item.Key || json.Unmarshal(b.filesJSON, &b.files) != nil || json.Unmarshal(b.reportJSON, &b.report) != nil {
		return miss("invalid identity or bundle metadata")
	}
	for _, f := range b.files {
		if len(f.SHA256) != 64 || f.Size < 0 || f.Size > 50_000_000 {
			return miss("invalid attachment metadata")
		}
		if decoded, decodeErr := hex.DecodeString(f.SHA256); decodeErr != nil || hex.EncodeToString(decoded) != f.SHA256 {
			return miss("invalid attachment hash")
		}
		info, statErr := os.Stat(filepath.Join(b.dir, "files", f.SHA256))
		if errors.Is(statErr, os.ErrNotExist) {
			return miss("attachment missing")
		}
		if statErr != nil {
			return nil, false, fmt.Errorf("inspect baseline attachment: %w", statErr)
		}
		if !info.Mode().IsRegular() || info.Size() != f.Size {
			return miss("attachment size or type mismatch")
		}
	}
	b.observations, err = q.ListItemFetchObservations(ctx, store.ListItemFetchObservationsParams{RunID: row.RunID, ItemKey: row.ItemKey, Generation: row.Generation, Attempt: row.Attempt})
	if err != nil {
		return nil, false, fmt.Errorf("read baseline observations: %w", err)
	}
	return b, false, nil
}

func adopted(o *store.RunFetchObservation) stage.FetchObservation {
	ref, _ := json.Marshal([5]any{o.RunID, o.ItemKey, o.Generation, o.Attempt, o.ResourceKey})
	origin := string(ref)
	var key [3]json.RawMessage
	var name *string
	if json.Unmarshal([]byte(o.ResourceKey), &key) == nil && string(key[1]) != "null" {
		var n string
		if json.Unmarshal(key[1], &n) == nil {
			name = &n
		}
	}
	return stage.FetchObservation{ResourceKey: o.ResourceKey, Source: o.Source, Name: name, URL: o.Url, Path: o.Path, ContentSHA256: o.ContentSha256, Size: o.Size, Acquisition: "adopted", FetchedAt: o.FetchedAt, OriginRef: &origin}
}

func networkObservation(endpoint, path string, data []byte, at time.Time) stage.FetchObservation {
	key, _ := json.Marshal([3]any{endpoint, nil, nil})
	return stage.FetchObservation{ResourceKey: string(key), Source: "vbpl", Path: path, ContentSHA256: digest(data), Size: int64(len(data)), Acquisition: "network", FetchedAt: &at}
}
