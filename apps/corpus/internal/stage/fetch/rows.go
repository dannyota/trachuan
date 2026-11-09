package fetch

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type detailEnvelope struct {
	Data struct {
		ID, DocNum, Title, AgencyName, IssueDate, EffFrom, EffTo string
		DocType, EffStatus                                       struct{ Code, Name string }
		IsConsolidatedDocument, HasContent                       bool
		DocumentContent                                          struct{ Content string }
	} `json:"data"`
}

// Rows prepares artifact rows solely from the published bundle.
func (*Stage) Rows(_ context.Context, env stage.Env, art artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	data, err := os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "detail.json"))
	if err != nil {
		return nil, fmt.Errorf("read detail: %w", err)
	}
	var detail detailEnvelope
	if err = json.Unmarshal(data, &detail); err != nil {
		return nil, fmt.Errorf("decode detail: %w", err)
	}
	if detail.Data.ID == "" {
		return nil, fmt.Errorf("detail has no document id")
	}
	data, err = os.ReadFile(filepath.Join(env.Artifacts.Abs(art.Path), "files.json"))
	if err != nil {
		return nil, fmt.Errorf("read files: %w", err)
	}
	var files []file
	if err = json.Unmarshal(data, &files); err != nil {
		return nil, fmt.Errorf("decode files: %w", err)
	}
	d := detail.Data
	return func(ctx context.Context, tx pgx.Tx, artifactID int64) error {
		q := store.New(tx)
		err := q.InsertDocument(ctx, store.InsertDocumentParams{ArtifactID: artifactID, Key: "vn:vbpl:" + d.ID, VbplID: d.ID, Number: d.DocNum, Title: d.Title, DocType: d.DocType.Name, Issuer: d.AgencyName, IssueDate: d.IssueDate, EffFrom: d.EffFrom, EffTo: d.EffTo, StatusCode: d.EffStatus.Code, StatusName: d.EffStatus.Name, IsConsolidated: d.IsConsolidatedDocument, HasContent: d.HasContent || d.DocumentContent.Content != ""})
		if err != nil {
			return fmt.Errorf("insert document: %w", err)
		}
		for _, f := range files {
			if insertErr := q.InsertFetchedFile(ctx, store.InsertFetchedFileParams{ArtifactID: artifactID, Name: f.Name, Sha256: f.SHA256, Size: f.Size, Path: "files/" + f.SHA256}); insertErr != nil {
				return fmt.Errorf("insert fetched file: %w", insertErr)
			}
		}
		return nil
	}, nil
}
