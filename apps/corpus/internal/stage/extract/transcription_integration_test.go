//go:build integration

package extract

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestExtract_HTMLTranscriptionArtifactIntegration(t *testing.T) {
	ctx := context.Background()
	db, err := pgxpool.New(ctx, testdb.New(t))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	data, err := os.ReadFile(filepath.Join("..", "..", "textparse", "testdata", "second.html"))
	if err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	artifacts := artifact.Store{Root: t.TempDir()}
	fetchArt, err := artifacts.PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error {
		if mkdirErr := os.Mkdir(filepath.Join(dir, "files"), 0700); mkdirErr != nil {
			return mkdirErr
		}
		files := fmt.Sprintf(`[{"name":"1_content.html","sha256":"%s","size":%d}]`, hash, len(data))
		for name, body := range map[string][]byte{"tree.json": []byte(`{"success":true,"data":[]}`), "detail.json": []byte(`{"data":{"id":"1"}}`), "files.json": []byte(files), filepath.Join("files", hash): data} {
			if writeErr := os.WriteFile(filepath.Join(dir, name), body, 0600); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID, artifactID int64
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn','20990105','running','{}') RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','vn:vbpl:1',$1,$2,$3,$4) RETURNING id`, fetchArt.InputHash, strings.Repeat("a", 64), fetchArt.Path, fetchArt.SHA256).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES ($1,'fetch','vn:vbpl:1','done',0,$2)`, runID, artifactID); err != nil {
		t.Fatal(err)
	}
	env := stage.Env{DB: db, Artifacts: artifacts}
	out, err := New().Process(ctx, env, stage.Run{ID: runID}, stage.Item{Key: "vn:vbpl:1", InputHash: InputHash(fetchArt.SHA256, "html_transcription")})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"text.json", "provisions.json", "report.json"} {
		if _, err = os.Stat(filepath.Join(artifacts.Abs(out.Artifact.Path), name)); err != nil {
			t.Fatal(err)
		}
	}
}
