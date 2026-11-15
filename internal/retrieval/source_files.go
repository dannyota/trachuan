package retrieval

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

type SourceFile struct {
	Name      string     `json:"name"`
	URL       *string    `json:"url"`
	Hash      string     `json:"hash"`
	Size      int64      `json:"size"`
	Role      string     `json:"role"`
	FetchedAt *time.Time `json:"fetched_at"`
}

func readSourceFiles(ctx context.Context, tx pgx.Tx, documentID int64) ([]SourceFile, error) {
	rows, err := tx.Query(ctx, `SELECT name,url,hash,size,role,fetched_at FROM source_file WHERE document_id=$1 ORDER BY name,hash`, documentID)
	if err != nil {
		return nil, databaseFailure("query source files", err)
	}
	defer rows.Close()
	files := []SourceFile{}
	for rows.Next() {
		var file SourceFile
		if err = rows.Scan(&file.Name, &file.URL, &file.Hash, &file.Size, &file.Role, &file.FetchedAt); err != nil {
			return nil, databaseFailure("scan source file", err)
		}
		files = append(files, file)
	}
	if err = rows.Err(); err != nil {
		return nil, databaseFailure("read source files", err)
	}
	return files, nil
}
