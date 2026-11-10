package discover

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/run"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Expected identifies a document and an optional reason to skip fetching it.
type Expected struct {
	Key, VBPLID string
	Skip        *stage.Gap
}

type discoveryRun struct {
	rel        string
	identities map[string]string
}

// ExpectedSet returns current and previously discovered documents in key order.
func ExpectedSet(ctx context.Context, db *pgxpool.Pool, r stage.Run, stores ...artifact.Store) ([]Expected, error) {
	q := store.New(db)
	list, err := source.Parse(r.Snapshot.SourceList)
	if err != nil {
		return nil, fmt.Errorf("parse discovery sources: %w", err)
	}
	if list.VBPL.SelectionVersion == 1 {
		return expectedMembership(ctx, q, r, stores...)
	}
	runs, err := q.ListDiscoveryRuns(ctx, r.Country)
	if err != nil {
		return nil, fmt.Errorf("list discovery runs: %w", err)
	}
	listings, err := q.ListDiscoveryListings(ctx, r.Country)
	if err != nil {
		return nil, fmt.Errorf("list discovery listings: %w", err)
	}
	items, err := q.ListDiscoveryItems(ctx, r.Country)
	if err != nil {
		return nil, fmt.Errorf("list discovery items: %w", err)
	}
	current, err := discoveryIdentities(r)
	if err != nil {
		return nil, err
	}
	history := make(map[int64]discoveryRun)
	for _, row := range runs {
		if run.CompareRel(row.Rel, r.Rel) >= 0 {
			continue
		}
		var snapshot stage.Snapshot
		if err := json.Unmarshal(row.Snapshot, &snapshot); err != nil {
			return nil, fmt.Errorf("decode snapshot for %s: %w", row.Rel, err)
		}
		identities, err := discoveryIdentities(stage.Run{Snapshot: snapshot})
		if err != nil {
			return nil, fmt.Errorf("discovery queries for %s: %w", row.Rel, err)
		}
		history[row.ID] = discoveryRun{rel: row.Rel, identities: identities}
	}
	complete := make(map[int64]map[string]bool)
	for _, item := range items {
		if complete[item.RunID] == nil {
			complete[item.RunID] = make(map[string]bool)
		}
		complete[item.RunID][item.ItemKey] = item.Complete
	}
	byRun := make(map[int64]map[string]map[string]bool)
	for _, listing := range listings {
		if englishRendition(listing.ItemKey) {
			continue
		}
		if byRun[listing.RunID] == nil {
			byRun[listing.RunID] = make(map[string]map[string]bool)
		}
		if byRun[listing.RunID][listing.QueryKey] == nil {
			byRun[listing.RunID][listing.QueryKey] = make(map[string]bool)
		}
		byRun[listing.RunID][listing.QueryKey][listing.ItemKey] = true
	}
	// Retain every current listing, including unions from incomplete queries.
	documents := make(map[string]map[string]bool)
	for _, keys := range byRun[r.ID] {
		for key := range keys {
			documents[key] = make(map[string]bool)
		}
	}
	latest := make(map[string]int64)
	for id, h := range history {
		for identity, itemKey := range h.identities {
			if !complete[id][itemKey] {
				continue
			}
			if previous, ok := latest[identity]; !ok || run.CompareRel(h.rel, history[previous].rel) > 0 {
				latest[identity] = id
			}
			for key := range byRun[id][identity] {
				if documents[key] == nil {
					documents[key] = make(map[string]bool)
				}
				documents[key][identity] = true
			}
		}
	}
	result := make([]Expected, 0, len(documents))
	for key, identities := range documents {
		prefix := r.Country + ":vbpl:"
		if !strings.HasPrefix(key, prefix) || len(key) == len(prefix) {
			return nil, fmt.Errorf("invalid listing document key %q", key)
		}
		entry := Expected{Key: key, VBPLID: strings.TrimPrefix(key, prefix)}
		present := false
		for _, keys := range byRun[r.ID] {
			present = present || keys[key]
		}
		if !present {
			remaining, withdrawn := 0, true
			for identity := range identities {
				itemKey, ok := current[identity]
				if !ok {
					continue
				}
				remaining++
				previous, ok := latest[identity]
				if !complete[r.ID][itemKey] || !ok || byRun[previous][identity][key] {
					withdrawn = false
				}
			}
			if remaining == 0 {
				entry.Skip = &stage.Gap{Kind: "scope_removed", Detail: key}
			} else if withdrawn {
				entry.Skip = &stage.Gap{Kind: "withdrawn", Detail: key}
			}
		}
		result = append(result, entry)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result, nil
}

func discoveryIdentities(r stage.Run) (map[string]string, error) {
	definitions, err := queries(r)
	if err != nil {
		return nil, err
	}
	identities := make(map[string]string, len(definitions))
	for key, definition := range definitions {
		identity, err := queryIdentity(key, definition)
		if err != nil {
			return nil, err
		}
		identities[identity] = key
	}
	return identities, nil
}

// englishRendition reports VBPL English translations (ids starting with
// vbpqta_). They are not legal text in Vietnamese, so they are never expected.
func englishRendition(key string) bool {
	return strings.Contains(key, ":vbpl:vbpqta_")
}

func expectedMembership(ctx context.Context, q *store.Queries, r stage.Run, stores ...artifact.Store) ([]Expected, error) {
	phase := membership.Seeds
	for _, name := range r.Snapshot.Stages {
		if name == "expand" {
			phase = membership.Expanded
			break
		}
	}
	snapshot, err := membership.ReadPhase(ctx, q, r.ID, phase)
	if err != nil {
		return nil, fmt.Errorf("read expected membership: %w", err)
	}
	active := make(map[string]bool, len(snapshot.Active))
	for _, entry := range snapshot.Active {
		active[entry.DocumentKey] = true
	}
	result := make([]Expected, 0, len(snapshot.Entries))
	for _, entry := range snapshot.Entries {
		if entry.Disposition == "source_replaced" {
			continue
		}
		if entry.Disposition != "active" && entry.Disposition != "withdrawn" && entry.Disposition != "scope_removed" {
			return nil, fmt.Errorf("unknown membership disposition %q", entry.Disposition)
		}
		e := Expected{Key: entry.DocumentKey, VBPLID: entry.SourceID}
		if entry.Disposition == "active" && entry.Source == "chinhphu" && len(stores) > 0 {
			candidates, err := currentHandOverCandidates(ctx, q, stores[0], r.ID, entry)
			if err != nil {
				return nil, err
			}
			e.Skip = precedenceGap(entry.DocumentKey, candidates)
			if e.Skip != nil && !active[strings.TrimPrefix(e.Skip.Detail, entry.DocumentKey+" -> ")] {
				e.Skip = nil
			}
		}
		if entry.Disposition != "active" {
			e.Skip = &stage.Gap{Kind: entry.Disposition, Detail: entry.DocumentKey}
		}
		result = append(result, e)
	}
	return result, nil
}

type handOverCandidate struct {
	record HandOverRecord
}

func precedenceGap(key string, candidates []handOverCandidate) *stage.Gap {
	target := ""
	var number, issuer, date string
	for _, c := range candidates {
		r := c.record
		matches, unresolved := 0, false
		for _, pair := range r.Comparisons {
			if pair.ID == "" || pair.Number == nil || retrieval.NormalizeNumber(*pair.Number) == "" {
				unresolved = true
				continue
			}
			candidateNumber, pairNumber := retrieval.NormalizeNumber(r.Number), retrieval.NormalizeNumber(*pair.Number)
			switch pair.State {
			case "match":
				matches++
				if pair.ID != r.PrimaryID || candidateNumber == "" || pairNumber != candidateNumber || pair.Issuer == nil || pair.Date == nil || source.NormalizeIssuer(r.Issuer) == "" || source.NormalizeIssuer(*pair.Issuer) != source.NormalizeIssuer(r.Issuer) || dateValue(r.Date) == "" || dateValue(*pair.Date) != dateValue(r.Date) {
					unresolved = true
				}
			case "different_date":
				if candidateNumber == "" || pairNumber != candidateNumber || pair.Date == nil || dateValue(r.Date) == "" || dateValue(*pair.Date) == "" || dateValue(r.Date) == dateValue(*pair.Date) {
					unresolved = true
				}
			case "different_number":
				if candidateNumber == "" || pairNumber == candidateNumber {
					unresolved = true
				}
			default:
				unresolved = true
			}
		}
		if r.State != "seed_primary" || r.ProbeState != "complete" || r.Number == "" || r.Issuer == "" || r.Date == "" || r.PrimaryID == "" || matches != 1 || unresolved {
			return nil
		}
		if target != "" && (target != r.PrimaryID || number != retrieval.NormalizeNumber(r.Number) || issuer != source.NormalizeIssuer(r.Issuer) || date != dateValue(r.Date)) {
			return nil
		}
		target = r.PrimaryID
		number, issuer, date = retrieval.NormalizeNumber(r.Number), source.NormalizeIssuer(r.Issuer), dateValue(r.Date)
	}
	if target == "" {
		return nil
	}
	return &stage.Gap{Kind: "source_replaced", Detail: key + " -> vn:vbpl:" + target}
}

func currentHandOverCandidates(ctx context.Context, q *store.Queries, artifacts artifact.Store, runID int64, entry membership.Entry) ([]handOverCandidate, error) {
	id, err := strconv.Atoi(entry.SourceID)
	if err != nil || id <= 0 || strconv.Itoa(id) != entry.SourceID {
		return nil, fmt.Errorf("invalid secondary id %q", entry.SourceID)
	}
	var result []handOverCandidate
	seen := map[string]bool{}
	for _, support := range entry.Supports {
		if support.RuleKind != "secondary_probe" {
			continue
		}
		key, docid, ok := strings.Cut(support.RuleValue, "|")
		if !ok || docid != entry.SourceID || seen[key] {
			continue
		}
		seen[key] = true
		row, err := q.GetRunItemArtifact(ctx, store.GetRunItemArtifactParams{RunID: runID, Stage: "discover", ItemKey: key})
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("read discovery item: %w", err)
		}
		if row.State != "done" {
			continue
		}
		dir := artifacts.Abs(row.Path)
		name := "candidate-" + entry.SourceID + ".json"
		if _, err = os.Stat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return nil, fmt.Errorf("inspect candidate: %w", err)
		}
		hash, err := artifact.VerifyFiles(dir, name)
		if err != nil {
			return nil, fmt.Errorf("verify candidate artifact: %w", err)
		}
		if hash != row.Sha256 {
			return nil, fmt.Errorf("candidate artifact hash mismatch")
		}
		record, err := ReadHandOverRecord(dir, entry.SourceID)
		if err != nil {
			return nil, fmt.Errorf("read candidate: %w", err)
		}
		if hash == support.Evidence.ArtifactHash && (record.Path != support.Evidence.Path || record.SHA256 != support.Evidence.ResponseSHA256) {
			return nil, fmt.Errorf("discovery candidate evidence mismatch")
		}
		if record.DetailPath == "" || len(record.Files) < 2 {
			record.ProbeState = "incomplete"
			result = append(result, handOverCandidate{record: record})
			continue
		}
		for _, file := range record.Files {
			verified, err := artifact.VerifyFiles(dir, file.Path)
			if err != nil {
				return nil, fmt.Errorf("verify discovery response: %w", err)
			}
			if verified != hash {
				return nil, fmt.Errorf("discovery manifest changed")
			}
			data, err := os.ReadFile(filepath.Join(dir, file.Path))
			if err != nil {
				return nil, fmt.Errorf("read discovery response: %w", err)
			}
			if fmt.Sprintf("%x", sha256.Sum256(data)) != file.SHA256 {
				return nil, fmt.Errorf("discovery response hash mismatch")
			}
		}
		result = append(result, handOverCandidate{record: record})
	}
	return result, nil
}
