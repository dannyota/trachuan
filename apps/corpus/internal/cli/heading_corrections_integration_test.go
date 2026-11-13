//go:build integration

package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage/extract"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestHeadingCorrections_RecordedRun(t *testing.T) {
	ctx := context.Background()
	url := testdb.New(t)
	db, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	t.Setenv("CORPUS_DATABASE_URL", url)
	root := t.TempDir()
	t.Setenv("CORPUS_ARTIFACTS", root)
	data, err := os.ReadFile("../stage/extract/testdata/heading-5664.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Tree json.RawMessage `json:"tree"`
		HTML string          `json:"html"`
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(f.HTML)))
	art, err := (artifact.Store{Root: root}).PublishContent(ctx, "fetch", strings.Repeat("a", 64), func(dir string) error {
		if e := os.Mkdir(filepath.Join(dir, "files"), 0700); e != nil {
			return e
		}
		for name, value := range map[string][]byte{"tree.json": f.Tree, "detail.json": []byte(`{"data":{"id":"5664"}}`), "files.json": fmt.Appendf(nil, `[{"name":"5664_content.html","sha256":"%s","size":%d}]`, hash, len(f.HTML)), "files/" + hash: []byte(f.HTML)} {
			if e := os.WriteFile(filepath.Join(dir, name), value, 0600); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var runID, artifactID int64
	if err = db.QueryRow(ctx, `INSERT INTO run(country,rel,state,snapshot) VALUES('vn','20990105','running','{}') RETURNING id`).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES('fetch','vn:vbpl:5664',$1,$2,$3,$4) RETURNING id`, art.InputHash, strings.Repeat("a", 64), art.Path, art.SHA256).Scan(&artifactID); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) VALUES($1,'fetch','vn:vbpl:5664','done',0,$2)`, runID, artifactID); err != nil {
		t.Fatal(err)
	}
	corrections := filepath.Join(t.TempDir(), "corrections.yaml")
	for _, quote := range []string{"Quyết định này có hiệu lực thi hành", "Quyết định này có hiệu lực  thi hành"} {
		data = fmt.Appendf(nil, "- key: vn:vbpl:2145\n  evidence: {key: 'vn:vbpl:5664', anchor: dieu-2/khoan-1, quote: '%s'}\n", quote)
		if err = os.WriteFile(corrections, data, 0600); err != nil {
			t.Fatal(err)
		}
		var out, errout bytes.Buffer
		code := Run(ctx, []string{"heading-corrections", "vn", "20990105", "--corrections", corrections}, &out, &errout)
		var checks []extract.HeadingCorrectionCheck
		if err = json.Unmarshal(out.Bytes(), &checks); err != nil {
			t.Fatalf("invalid report; stderr=%s: %v", &errout, err)
		}
		found := !strings.Contains(quote, "  ")
		if len(checks) != 1 || checks[0].EvidenceKey != "vn:vbpl:5664" || checks[0].QuoteFound != found || (code == 0) != found {
			t.Fatalf("code=%d checks=%+v stderr=%s", code, checks, &errout)
		}
	}
	var out, errout bytes.Buffer
	code := Run(ctx, []string{"heading-corrections", "vn", "20990105", "--corrections", filepath.Join(t.TempDir(), "absent.yaml")}, &out, &errout)
	if code != 0 || strings.TrimSpace(out.String()) != "[]" {
		t.Fatalf("missing corrections file: code=%d stdout=%s stderr=%s", code, &out, &errout)
	}

}
