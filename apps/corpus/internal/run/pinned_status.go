package run

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/pinned"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type WithdrawnNamed struct {
	Key     string `json:"key"`
	Number  string `json:"number"`
	Rel     string `json:"rel,omitempty"`
	Expand  string `json:"expand,omitempty"`
	Fetch   string `json:"fetch,omitempty"`
	Message string `json:"message,omitempty"`
}

func newlyWithdrawnNamed(current, prior []membership.Entry) []WithdrawnNamed {
	removed := map[string]bool{}
	for _, entry := range prior {
		if entry.Disposition == "withdrawn" {
			removed[entry.DocumentKey] = true
		}
	}
	result := []WithdrawnNamed{}
	for _, entry := range current {
		if entry.Disposition != "withdrawn" || removed[entry.DocumentKey] {
			continue
		}
		named := false
		for _, support := range entry.Supports {
			named = named || support.RuleKind == "named"
		}
		if !named {
			continue
		}
		row := WithdrawnNamed{Key: entry.DocumentKey}
		if entry.RawNumber != nil {
			row.Number = *entry.RawNumber
		}
		result = append(result, row)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}

func (r *Report) loadPinStatus(ctx context.Context, q *store.Queries, artifacts artifact.Store) error {
	r.NewlyWithdrawnNamed = []WithdrawnNamed{}
	r.PinNotices = []pinned.Notice{}
	scope, err := q.GetRunScope(ctx, r.Run.ID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read pin status scope: %w", err)
	}
	if err == nil && scope.SeedsSnapshot != nil {
		current, phaseErr := membership.ReadPhase(ctx, q, r.Run.ID, membership.Seeds)
		if phaseErr != nil {
			return phaseErr
		}
		var prior []membership.Entry
		if scope.PredecessorRunID != nil && scope.PredecessorPhase != nil && *scope.PredecessorPhase == "final" {
			previous, priorErr := membership.ReadPhase(ctx, q, *scope.PredecessorRunID, membership.Final)
			if priorErr != nil {
				return priorErr
			}
			prior = previous.Entries
		}
		r.NewlyWithdrawnNamed = newlyWithdrawnNamed(current.Entries, prior)
		for i := range r.NewlyWithdrawnNamed {
			if err = r.loadNamedAcquisitions(ctx, q, artifacts, &r.NewlyWithdrawnNamed[i]); err != nil {
				return err
			}
		}
	}
	var snapshot stage.Snapshot
	if err = json.Unmarshal(r.Run.Snapshot, &snapshot); err != nil {
		return fmt.Errorf("decode pin status snapshot: %w", err)
	}
	list, err := source.Parse(snapshot.SourceList)
	if err != nil {
		return fmt.Errorf("parse pin status sources: %w", err)
	}
	if len(list.VBPL.Pinned) == 0 {
		return nil
	}
	items, err := q.ListStageItemArtifacts(ctx, store.ListStageItemArtifactsParams{RunID: r.Run.ID, Stage: "discover"})
	if err != nil {
		return fmt.Errorf("read pin status items: %w", err)
	}
	notices := map[string]bool{}
	for _, item := range items {
		if item.State != "done" || item.Path == nil || item.Sha256 == nil {
			continue
		}
		dir := artifacts.Abs(*item.Path)
		hash, err := artifact.Verify(dir)
		if err != nil || hash != *item.Sha256 {
			return fmt.Errorf("invalid discovery notice artifact for %s", item.ItemKey)
		}
		name := "report.json"
		if strings.HasPrefix(item.ItemKey, "chinhphu:") {
			name = "secondary.json"
		}
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return fmt.Errorf("read discovery notices: %w", err)
		}
		var report struct {
			Notices []pinned.Notice `json:"notices"`
		}
		if err = json.Unmarshal(data, &report); err != nil {
			return fmt.Errorf("decode discovery notices: %w", err)
		}
		for _, notice := range report.Notices {
			if notice.Kind == "source_pin_live" {
				notices[notice.Key] = true
			}
		}
	}
	for key := range notices {
		r.PinNotices = append(r.PinNotices, pinned.Notice{Kind: "source_pin_live", Key: key})
	}
	sort.Slice(r.PinNotices, func(i, j int) bool { return r.PinNotices[i].Key < r.PinNotices[j].Key })
	return nil
}

func (r *Report) loadNamedAcquisitions(ctx context.Context, q *store.Queries, artifacts artifact.Store, doc *WithdrawnNamed) error {
	rows, err := q.ListDocumentAcquisitions(ctx, store.ListDocumentAcquisitionsParams{Country: r.Run.Country, ItemKey: doc.Key})
	if err != nil {
		return fmt.Errorf("read named acquisitions: %w", err)
	}
	sort.Slice(rows, func(i, j int) bool { return CompareRel(rows[i].Rel, rows[j].Rel) > 0 })
	candidate := WithdrawnNamed{}
	for _, row := range rows {
		if row.Rel == "" {
			continue
		}
		if candidate.Rel != row.Rel {
			candidate = WithdrawnNamed{Key: doc.Key, Number: doc.Number, Rel: row.Rel}
		}
		number, verified := verifiedNamedAcquisition(artifacts, r.Run.Country, doc.Key, row)
		if !verified {
			continue
		}
		switch row.Stage {
		case "expand":
			candidate.Expand = row.Sha256
		case "fetch":
			candidate.Fetch = row.Sha256
			candidate.Number = number
		}
		if candidate.Expand != "" && candidate.Fetch != "" {
			*doc = candidate
			return nil
		}
	}
	*doc = WithdrawnNamed{Key: doc.Key, Number: doc.Number, Message: "no verified pair"}
	return nil
}

func verifiedNamedAcquisition(artifacts artifact.Store, country, key string, row store.ListDocumentAcquisitionsRow) (string, bool) {
	number, err := pinned.InspectArtifact(artifacts, country, key, store.GetPinnedArtifactRow{Stage: row.Stage, Path: row.Path, Sha256: row.Sha256})
	return number, err == nil
}
