package build

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
)

func (l *loader) loadSourceFiles(ctx context.Context, key, source, dir string, id int64, files []fetch.DocumentFile) error {
	missing := map[string]bool{}
	if l.inv.Version != 2 {
		missing["url"] = true
		missing["role"] = true
		missing["fetched_at"] = true
	}
	for index, file := range files {
		if index > 0 && files[index-1].Name == file.Name && files[index-1].SHA256 == file.SHA256 {
			continue
		}
		if source != "chinhphu" {
			if err := checkSourceFile(filepath.Join(dir, "files", file.SHA256), file.Size, file.SHA256); err != nil {
				return err
			}
		}
		var url *string
		role := "unknown"
		var fetched *time.Time
		if l.inv.Version == 2 {
			found := false
			for _, p := range l.provenanceFiles[key] {
				if p.DocumentKey == key && p.Source == source && p.Name == file.Name && p.SHA256 == file.SHA256 && p.Size == file.Size && p.Path == "files/"+file.SHA256 {
					found = true
					url = p.URL
					fetched = p.FetchedAt
					if p.Role != nil {
						role = *p.Role
					}
					break
				}
			}
			if !found {
				return fmt.Errorf("missing provenance for %s file %s", key, file.Name)
			}
			if url == nil {
				missing["url"] = true
			}
			if role == "unknown" {
				missing["role"] = true
			}
			if fetched == nil {
				missing["fetched_at"] = true
			}
		}
		err := l.batch.queue(ctx, "insert source file", `INSERT INTO source_file(id,document_id,source,name,hash,size,url,role,fetched_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, l.counts.SourceFiles+1, id, source, file.Name, file.SHA256, file.Size, url, role, fetched)
		if err != nil {
			return err
		}
		l.counts.SourceFiles++
	}
	var fields []string
	for _, field := range []string{"url", "role", "fetched_at"} {
		if missing[field] {
			fields = append(fields, field)
		}
	}
	if len(fields) > 0 {
		l.addGap("build", key, "source_provenance_missing", strings.Join(fields, ","))
	}
	return nil
}
