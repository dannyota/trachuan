package build

import (
	"context"
	"sort"

	"danny.vn/trachuan/internal/frameworks/registry"
	"github.com/jackc/pgx/v5/pgconn"
)

type sourceFileDB interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func loadSourceFiles(ctx context.Context, tx sourceFileDB, d databaseInputs, ids databaseIDs) error {
	id := 0
	for _, v := range d.versions {
		files := append([]registry.File{}, v.Files...)
		sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
		for _, file := range files {
			switch file.Role {
			case "catalog", "text", "mapping workbook":
			default:
				continue
			}
			id++
			role := file.Role
			if role == "mapping workbook" {
				role = "mapping"
			}
			//nolint:misspell // Match the registry field names.
			_, err := tx.Exec(ctx, `INSERT INTO source_file(id,version_id,name,role,sha256,size,licence_kind,text_source,text_complete) VALUES($1,$2,$3,$4,$5,$6,$7,'structured',true)`, id, ids.versions[versionKey(v.Code, v.Version)], file.Path, role, file.SHA256, file.Size, v.LicenceKind)
			if err != nil {
				return hide("load source file", err)
			}
		}
	}
	return nil
}
