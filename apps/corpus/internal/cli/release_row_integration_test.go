//go:build integration

package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestReleaseRow_ReadOnlyMetadataAndAcceptedGaps(t *testing.T) {
	ctx := context.Background()
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = admin.Close(ctx) }()
	name, key, err := testpg.CreateCorpus(ctx, admin, t.Name())
	if err != nil {
		t.Fatal(err)
	}
	defer testpg.DropCorpusDatabase(t, admin, name, key)
	base.Path = "/" + name
	builderURL := base.String()
	corpus, err := pgxpool.New(ctx, builderURL)
	if err != nil {
		t.Fatal("open synthetic corpus")
	}
	defer corpus.Close()
	rel := strings.TrimPrefix(key, "vn/")
	hash := strings.Repeat("a", 64)
	_, err = corpus.Exec(ctx, `CREATE EXTENSION vector; CREATE EXTENSION ltree;
	CREATE TABLE release (singleton boolean, country text, rel text, schema_version int,
	dense_recipe jsonb, chunk_recipe jsonb, lexical_recipe jsonb, retrieval_recipe jsonb,
	taxonomy_version text, source_manifest_hash text, build_input_hash text,
	build_recipe_hash text, evaluation_summary jsonb, lexical_stats jsonb, built_at timestamptz)`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = corpus.Exec(ctx, `INSERT INTO release VALUES (true,'vn',$1,7,
	'{"document":{},"query":{}}','{"version":1}','{}','{"version":3}',
	'',$2,$2,$2,'{"status":"not_run","release_eligible":false}','{}','2099-01-01T00:00:00Z')`, rel, hash)
	if err != nil {
		t.Fatal(err)
	}
	workshopURL := testdb.New(t)
	workshop, err := pgxpool.New(ctx, workshopURL)
	if err != nil {
		t.Fatal("open synthetic workshop")
	}
	defer workshop.Close()
	artifacts := artifact.Store{Root: t.TempDir()}
	reportBody := []byte(`{"status":"complete"}`)
	evalArtifact, err := artifacts.Publish(ctx, "eval", hash, hash, func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "report.json"), reportBody, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	evidenceArtifact, err := artifacts.Publish(ctx, "evidence", hash, hash, func(dir string) error {
		for name, body := range evidenceFiles {
			if writeErr := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); writeErr != nil {
				return writeErr
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID, artifactID, evalID, evidenceID int64
	if err = workshop.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES ('vn',$1,'evaluated','{"PreviousReport":{"sha256":"none"}}') RETURNING id`, rel).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err = workshop.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('build',$1,$2,$2,'synthetic',$2) RETURNING id`, key, hash).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	if err = workshop.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('eval',$1,$2,$2,$3,$4) RETURNING id`, key, hash, evalArtifact.Path, evalArtifact.SHA256).Scan(&evalID); err != nil {
		t.Fatal(err)
	}
	if err = workshop.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('evidence',$1,$2,$2,$3,$4) RETURNING id`, key, hash, evidenceArtifact.Path, evidenceArtifact.SHA256).Scan(&evidenceID); err != nil {
		t.Fatal(err)
	}
	_, err = workshop.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,input_hash,recipe_hash,state,batch,artifact_id)
	VALUES ($1,'build',$2,$3,$3,'done',0,$4),($1,'fetch','gap',$3,$3,'gap',0,NULL),($1,'eval',$2,$3,$3,'done',0,$5),($1,'evidence',$2,$3,$3,'done',0,$6)`, runID, key, hash, artifactID, evalID, evidenceID)
	if err != nil {
		t.Fatal(err)
	}
	read := func(want bool) {
		t.Helper()
		data, readErr := readReleaseRow(ctx, "vn", rel, builderURL, workshopURL, artifacts)
		if readErr != nil {
			t.Fatal(readErr)
		}
		var row struct {
			Rel        string            `json:"rel"`
			Extensions map[string]string `json:"extensions"`
			Run        struct {
				Complete             bool   `json:"complete"`
				EvalReportSHA256     string `json:"eval_report_sha256"`
				PreviousReportSHA256 string `json:"previous_report_sha256"`
				FindingsSHA256       string `json:"findings_sha256"`
				IdentitySHA256       string `json:"identity_sha256"`
				ChangesSHA256        string `json:"changes_sha256"`
			} `json:"run"`
		}
		if readErr = json.Unmarshal(data, &row); readErr != nil || row.Rel != rel || row.Run.Complete != want || len(row.Extensions) != 2 || row.Extensions["vector"] == "" || row.Extensions["ltree"] == "" {
			t.Fatalf("release=%s err=%v", data, readErr)
		}
		if row.Run.EvalReportSHA256 != sha256Hex(reportBody) || row.Run.PreviousReportSHA256 != "none" {
			t.Fatalf("report hashes in %s", data)
		}
		if row.Run.FindingsSHA256 != sha256Hex([]byte(evidenceFiles["findings.json"])) || row.Run.IdentitySHA256 != sha256Hex([]byte(evidenceFiles["identity.jsonl"])) || row.Run.ChangesSHA256 != sha256Hex([]byte(evidenceFiles["changes.jsonl"])) {
			t.Fatalf("evidence hashes in %s", data)
		}
	}
	read(false)
	if _, err = workshop.Exec(ctx, `INSERT INTO gap(run_id,stage,item_key,kind,detail) VALUES ($1,'fetch','gap','missing_attachment','synthetic')`, runID); err != nil {
		t.Fatal(err)
	}
	read(false)
	if _, err = workshop.Exec(ctx, `UPDATE gap SET accepted_at=now() WHERE run_id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	read(true)
	for _, state := range []string{"pending", "failed"} {
		if _, err = workshop.Exec(ctx, `UPDATE run_item SET state=$2 WHERE run_id=$1 AND stage='fetch'`, runID, state); err != nil {
			t.Fatal(err)
		}
		read(false)
	}
	if _, err = workshop.Exec(ctx, `UPDATE run_item SET state='done' WHERE run_id=$1 AND stage='fetch'`, runID); err != nil {
		t.Fatal(err)
	}
	read(true)
	if _, err = workshop.Exec(ctx, `DELETE FROM run_item WHERE run_id=$1 AND stage='evidence'`, runID); err != nil {
		t.Fatal(err)
	}
	if _, err = readReleaseRow(ctx, "vn", rel, builderURL, workshopURL, artifacts); err == nil || !strings.Contains(err.Error(), "no done evidence") {
		t.Fatalf("decided run without evidence: %v", err)
	}
	if _, err = workshop.Exec(ctx, `UPDATE run SET snapshot='{}' WHERE id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	if _, err = readReleaseRow(ctx, "vn", rel, builderURL, workshopURL, artifacts); err == nil || !strings.Contains(err.Error(), "previous report decision") {
		t.Fatalf("run without a decision above the cutoff: %v", err)
	}
	if _, err = workshop.Exec(ctx, `UPDATE run SET snapshot='{"PreviousReport":{"sha256":"none"}}' WHERE id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	if _, err = corpus.Exec(ctx, `UPDATE release SET build_input_hash=$1`, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = readReleaseRow(ctx, "vn", rel, builderURL, workshopURL, artifacts); err == nil {
		t.Fatal("release for another build accepted")
	}
	var count int
	if err = corpus.QueryRow(ctx, `SELECT count(*) FROM release`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("release changed count=%d err=%v", count, err)
	}
}
