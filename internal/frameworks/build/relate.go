package build

import (
	"context"
	"path/filepath"

	"danny.vn/trachuan/internal/frameworks/parse"
	"danny.vn/trachuan/internal/frameworks/relate"
)

func relateStep(ctx context.Context, base, dir string) error {
	snapshot := filepath.Join(base, "snapshot")
	reg, err := readRegistry(filepath.Join(snapshot, "sources/frameworks.yaml"))
	if err != nil {
		return err
	}
	rows, err := loadRows[catalog](filepath.Join(base, "parse/catalogs.jsonl"))
	if err != nil {
		return err
	}
	catalogs := map[string]*parse.Result{}
	for _, row := range rows {
		catalogs[row.Code+":"+row.Version] = &parse.Result{Nodes: row.Nodes}
	}
	var inputs []relate.Input
	var gaps []Gap
	for _, v := range releaseVersions(reg) {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		nodes := catalogs[v.Code+":"+v.Version].Nodes
		dropped := 0
		for _, file := range v.Files {
			if file.Role != "mapping workbook" {
				continue
			}
			data, readErr := readFile(filepath.Join(snapshot, "files", file.Path))
			if readErr != nil {
				return readErr
			}
			mappings, count, workbookErr := relate.Workbook(file.Parser, data, nodes)
			if workbookErr != nil {
				return hide("parse mapping workbook", workbookErr)
			}
			inputs = append(inputs, mappings...)
			dropped += count
		}
		if dropped > 0 {
			gaps = append(gaps, Gap{Kind: "mapping_dropped", Code: v.Code, Version: v.Version, Count: dropped})
		}
	}
	edges, stats, err := relate.Build(reg, catalogs, inputs)
	if err != nil {
		return hide("build edges", err)
	}
	for key, count := range stats.DroppedByVersion {
		gaps = append(gaps, Gap{Kind: "mapping_dropped", Code: key.Code, Version: key.Version, Count: count})
	}
	edges, gaps = releaseMappings(edges, gaps)
	if err := writeRows(dir, "edges.jsonl", edges); err != nil {
		return err
	}
	return writeGaps(dir, gaps)
}
