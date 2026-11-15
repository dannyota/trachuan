package build

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"

	"danny.vn/trachuan/internal/frameworks/parse"
	"danny.vn/trachuan/internal/frameworks/registry"
	"danny.vn/trachuan/internal/frameworks/relate"
)

type catalog struct {
	Code    string       `json:"code"`
	Version string       `json:"version"`
	Nodes   []parse.Node `json:"nodes"`
}

func parseStep(ctx context.Context, base, dir string) error {
	snapshot := filepath.Join(base, "snapshot")
	reg, err := readRegistry(filepath.Join(snapshot, "sources/frameworks.yaml"))
	if err != nil {
		return err
	}
	titles, err := readTitles(filepath.Join(snapshot, "trachuan/titles.csv"))
	if err != nil {
		return err
	}
	var controls []Control
	var catalogs []catalog
	var gaps []Gap
	for _, v := range releaseVersions(reg) {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := parseVersion(snapshot, v)
		if err != nil {
			return err
		}
		if err := validateLineage(result.Nodes); err != nil {
			return err
		}
		catalogs = append(catalogs, catalog{v.Code, v.Version, result.Nodes})
		var rows []Control
		for _, n := range result.Nodes {
			row := Control{Code: v.Code, Version: v.Version, Citation: n.Citation, CitationNorm: n.CitationNorm, Kind: n.Kind, Ordinal: n.Ordinal, Status: n.Status, TitleOriginal: n.PublisherTitle, Body: n.Body, Notes: []string{}, Tables: []string{}}
			if n.Parent >= 0 {
				parent := result.Nodes[n.Parent].Citation
				row.Parent = &parent
			}
			key := relate.Key{Code: v.Code, Version: v.Version, Citation: n.Citation}
			title, present := titles[key]
			if !present && v.Access == "free" {
				title = n.PublisherTitle
			}
			if title != "" {
				row.Title = &title
			} else if v.Access == "licensed" {
				citation := n.Citation
				gaps = append(gaps, Gap{Kind: "title_missing", Code: v.Code, Version: v.Version, Citation: &citation, Count: 1})
			}
			rows = append(rows, row)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].CitationNorm < rows[j].CitationNorm })
		controls = append(controls, rows...)
	}
	if err := writeRows(dir, "controls.jsonl", controls); err != nil {
		return err
	}
	if err := writeRows(dir, "catalogs.jsonl", catalogs); err != nil {
		return err
	}
	return writeGaps(dir, gaps)
}

func parseVersion(snapshot string, v registry.Framework) (*parse.Result, error) {
	var catalogs []registry.File
	for _, file := range v.Files {
		if file.Role == "catalog" {
			catalogs = append(catalogs, file)
		}
	}
	if len(catalogs) != 1 {
		return nil, fmt.Errorf("expected one catalog per release version")
	}
	data, err := readFile(filepath.Join(snapshot, "files", catalogs[0].Path))
	if err != nil {
		return nil, err
	}
	result, err := parse.Parse(v.Parser, data, v.Code, v.Version, v.ExpectedControls)
	if err != nil {
		return nil, hide("parse catalog", err)
	}
	return result, nil
}

func validateLineage(nodes []parse.Node) error {
	ordinals := map[int]bool{}
	for i, n := range nodes {
		if n.Ordinal < 0 || ordinals[n.Ordinal] {
			return fmt.Errorf("invalid control ordinal")
		}
		ordinals[n.Ordinal] = true
		seen := map[int]bool{i: true}
		for parent := n.Parent; parent != -1; parent = nodes[parent].Parent {
			if parent < 0 || parent >= len(nodes) || seen[parent] {
				return fmt.Errorf("invalid control lineage")
			}
			seen[parent] = true
		}
	}
	return nil
}
