package fetch

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/discover"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type handOverRecord struct {
	record   discover.HandOverRecord
	refs     []string
	complete bool
	current  bool
}

func readCandidateArtifact(env stage.Env, row store.GetRunItemArtifactRow, id string) (handOverRecord, error) {
	dir := env.Artifacts.Abs(row.Path)
	name := "candidate-" + id + ".json"
	hash, err := artifact.VerifyFiles(dir, name)
	if err != nil {
		return handOverRecord{}, fmt.Errorf("verify candidate: %w", err)
	}
	if hash != row.Sha256 {
		return handOverRecord{}, fmt.Errorf("candidate artifact hash mismatch")
	}
	record, err := discover.ReadHandOverRecord(dir, id)
	if err != nil {
		return handOverRecord{}, fmt.Errorf("read candidate: %w", err)
	}
	refs := []string{hash + "/" + name}
	complete := record.DetailPath != "" && len(record.Files) >= 2
	for _, file := range record.Files {
		actual, err := artifact.VerifyFiles(dir, file.Path)
		if err != nil {
			return handOverRecord{}, fmt.Errorf("verify candidate response: %w", err)
		}
		if actual != hash {
			return handOverRecord{}, fmt.Errorf("candidate manifest changed")
		}
		data, err := os.ReadFile(filepath.Join(dir, file.Path))
		if err != nil {
			return handOverRecord{}, fmt.Errorf("read candidate response: %w", err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != file.SHA256 {
			return handOverRecord{}, fmt.Errorf("candidate response hash mismatch")
		}
		refs = append(refs, hash+"/"+file.Path)
	}
	return handOverRecord{record: record, refs: refs, complete: complete}, nil
}
func readHandOverRecords(ctx context.Context, q *store.Queries, env stage.Env, run stage.Run, entry membership.Entry) ([]handOverRecord, error) {
	id, err := strconv.Atoi(entry.SourceID)
	if err != nil || id <= 0 || strconv.Itoa(id) != entry.SourceID {
		return nil, fmt.Errorf("invalid secondary id %q", entry.SourceID)
	}
	var out []handOverRecord
	seen := map[string]bool{}
	var earlier []store.ListEarlierRunsRow
	loaded := false
	for _, support := range entry.Supports {
		if support.RuleKind != "secondary_probe" {
			continue
		}
		key, docid, ok := strings.Cut(support.RuleValue, "|")
		if !ok || docid != entry.SourceID {
			continue
		}
		row, err := q.GetRunItemArtifact(ctx, store.GetRunItemArtifactParams{RunID: run.ID, Stage: "discover", ItemKey: key})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("read discovery item: %w", err)
		}
		if err == nil && row.State == "done" {
			if _, statErr := os.Stat(filepath.Join(env.Artifacts.Abs(row.Path), "candidate-"+entry.SourceID+".json")); statErr == nil {
				candidate, e := readCandidateArtifact(env, row, entry.SourceID)
				if e != nil {
					return nil, e
				}
				if row.Sha256 == support.Evidence.ArtifactHash && (candidate.record.Path != support.Evidence.Path || candidate.record.SHA256 != support.Evidence.ResponseSHA256) {
					return nil, fmt.Errorf("discovery candidate evidence mismatch")
				}
				candidate.current = true
				seen[row.Sha256] = true
				out = append(out, candidate)
			} else if !errors.Is(statErr, os.ErrNotExist) {
				return nil, fmt.Errorf("inspect candidate: %w", statErr)
			}
		}
		if support.Evidence.ArtifactHash == "" || seen[support.Evidence.ArtifactHash] {
			continue
		}
		if !loaded {
			earlier, err = q.ListEarlierRuns(ctx, store.ListEarlierRunsParams{Country: run.Country, ID: run.ID})
			if err != nil {
				return nil, fmt.Errorf("list earlier runs: %w", err)
			}
			loaded = true
		}
		found := false
		for _, old := range earlier {
			oldRow, e := q.GetRunItemArtifact(ctx, store.GetRunItemArtifactParams{RunID: old.ID, Stage: "discover", ItemKey: key})
			if errors.Is(e, pgx.ErrNoRows) {
				continue
			}
			if e != nil {
				return nil, fmt.Errorf("read retained candidate: %w", e)
			}
			if oldRow.Sha256 != support.Evidence.ArtifactHash {
				continue
			}
			candidate, e := readCandidateArtifact(env, oldRow, entry.SourceID)
			if e != nil {
				return nil, e
			}
			if candidate.record.Path != support.Evidence.Path || candidate.record.SHA256 != support.Evidence.ResponseSHA256 {
				return nil, fmt.Errorf("retained candidate evidence mismatch")
			}
			out = append(out, candidate)
			seen[oldRow.Sha256] = true
			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf("retained discovery artifact missing for %s", entry.DocumentKey)
		}
	}
	return out, nil
}
