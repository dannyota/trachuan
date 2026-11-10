package discover

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/lookup"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5"
)

func readJSON(dir, name string, v any) error {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if err = json.Unmarshal(data, v); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}
func resolveScope(env stage.Env, r stage.Run, it stage.Item, art artifact.Result, rules source.VBPLSources) (stage.Resolution, error) {
	dir := env.Artifacts.Abs(art.Path)
	var report scopeReport
	if err := readJSON(dir, "report.json", &report); err != nil {
		return stage.Resolution{}, err
	}
	var rows []selectedItem
	var gaps []stage.Gap
	identity := ""
	if it.Key == "vbpl:central" {
		identity = source.CentralListingIdentity("vbpl", "TRUNG_UONG", 1000)
		var selected selectedFile
		if err := readJSON(dir, "selected.json", &selected); err != nil {
			return stage.Resolution{}, err
		}
		rows = selected.Items
		if !report.Stable {
			gaps = append(gaps, stage.Gap{Kind: "listing_unstable", Detail: it.Key})
		}
	} else {
		raw := strings.TrimPrefix(it.Key, "vbpl:named:")
		found := false
		for _, entry := range rules.NamedNumbers {
			if it.Key == "vbpl:named:"+entry {
				found = true
				break
			}
		}
		if !found {
			return stage.Resolution{}, fmt.Errorf("unknown discovery query %q", it.Key)
		}
		entry, err := source.ParseNamedNumber(raw)
		if err != nil {
			return stage.Resolution{}, fmt.Errorf("parse named entry: %w", err)
		}
		identity = source.NamedLookupIdentity("vbpl", "docNum", "exact", "TRUNG_UONG", entry.Number)
		var result lookup.Result
		if err = readJSON(dir, "lookup.json", &result); err != nil {
			return stage.Resolution{}, err
		}
		if result.Evidence.QueryIdentity != identity {
			return stage.Resolution{}, fmt.Errorf("lookup query mismatch")
		}
		if !report.Stable || result.Evidence.Completeness != lookup.Complete {
			gaps = append(gaps, stage.Gap{Kind: "listing_unstable", Detail: raw})
		} else {
			eligible, unresolved := namedCandidates(result.Items, entry)
			if len(eligible) == 1 && len(unresolved) == 0 {
				item := eligible[0]
				ref := pageFor(result, item.PageRef, len(report.Passes))
				rows = append(rows, selectedItem{ID: item.SourceID, Number: item.RawNumber, Title: item.Title, IssueDate: item.IssueDate, Issuer: item.Issuer, AgencyIDs: item.AgencyIDs, Rules: []source.SelectionRule{{Kind: "named", Value: raw}}, Page: ref})
			} else if len(eligible) == 0 && len(unresolved) == 0 {
				gaps = append(gaps, stage.Gap{Kind: "named_document_missing", Detail: raw})
			} else {
				candidates := append(ids(eligible), ids(unresolved)...)
				sort.Strings(candidates)
				gaps = append(gaps, stage.Gap{Kind: "named_document_ambiguous", Detail: raw + ": " + strings.Join(candidates, ",")})
			}
		}
	}
	if report.Version != 2 || report.QueryIdentity != identity {
		return stage.Resolution{}, fmt.Errorf("discovery report query mismatch")
	}
	return stage.Resolution{Gaps: gaps, RunRows: func(ctx context.Context, tx pgx.Tx) error {
		q := store.New(tx)
		for _, row := range rows {
			payload, err := json.Marshal(row)
			if err != nil {
				return fmt.Errorf("encode listing: %w", err)
			}
			if err = q.UpsertListing(ctx, store.UpsertListingParams{RunID: r.ID, QueryKey: identity, ItemKey: r.Country + ":vbpl:" + row.ID, Payload: payload}); err != nil {
				return fmt.Errorf("upsert listing %q: %w", row.ID, err)
			}
		}
		return nil
	}}, nil
}
func namedCandidates(items []lookup.Item, entry source.NamedNumber) (eligible, unresolved []lookup.Item) {
	for _, item := range items {
		if strings.HasPrefix(item.SourceID, "vbpqta_") || item.RawNumber == nil || retrieval.NormalizeNumber(*item.RawNumber) != retrieval.NormalizeNumber(entry.Number) {
			continue
		}
		if item.IssueDate == nil || len(*item.IssueDate) < 10 {
			unresolved = append(unresolved, item)
			continue
		}
		date := (*item.IssueDate)[:10]
		if _, err := time.Parse("2006-01-02", date); err != nil {
			unresolved = append(unresolved, item)
			continue
		}
		if entry.Date == "" || date == entry.Date {
			eligible = append(eligible, item)
		}
	}
	return
}
func pageFor(result lookup.Result, hash string, pass int) pageReference {
	for _, p := range result.Evidence.PagesFetched {
		if p.SHA256 == hash && p.Ordinal != nil {
			return pageReference{Pass: pass, Page: *p.Ordinal, SHA256: hash, Path: p.Path}
		}
	}
	return pageReference{}
}
