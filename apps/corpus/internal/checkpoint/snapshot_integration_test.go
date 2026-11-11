//go:build integration

package checkpoint

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/testdb"
	"github.com/jackc/pgx/v5/pgxpool"
)

type detachingDumper struct {
	dump   PgDump
	detach func(context.Context) error
}

func (d detachingDumper) Dump(ctx context.Context, url, path, snapshot string) error {
	if err := d.dump.Dump(ctx, url, path, snapshot); err != nil {
		return err
	}
	return d.detach(ctx)
}

func TestTake_ReferenceDetachmentAfterDumpUsesDumpSnapshot(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		for _, detach := range []string{"delete", "reset"} {
			name := detach + "/valid"
			if corrupt {
				name = detach + "/corrupt"
			}
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				url := testdb.New(t)
				db, err := pgxpool.New(ctx, url)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				taker := setup(t)
				taker.beginSnapshot = nil
				taker.Config.DatabaseURL = url
				rel := publish(t, taker.Config.ArtifactsRoot, "snapshot artifact")
				sum, err := artifact.Verify(filepath.Join(taker.Config.ArtifactsRoot, rel))
				if err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec(ctx, `INSERT INTO run(country,rel,state,generation,snapshot) VALUES ('vn','20990111_1','running',1,'{}'),('vn','20990111_2','paused',1,'{}')`); err != nil {
					t.Fatal(err)
				}
				var id int64
				err = db.QueryRow(ctx, `INSERT INTO artifact(stage,item_key,input_hash,recipe_hash,path,sha256) VALUES ('fetch','item',$1,$2,$3,$4) RETURNING id`, strings.Repeat("a", 64), strings.Repeat("b", 64), rel, sum).Scan(&id)
				if err != nil {
					t.Fatal(err)
				}
				if _, err = db.Exec(ctx, `INSERT INTO run_item(run_id,stage,item_key,state,batch,artifact_id) SELECT id,'fetch','item','done',0,$1 FROM run WHERE rel='20990111_2'`, id); err != nil {
					t.Fatal(err)
				}
				if corrupt {
					if err = os.WriteFile(filepath.Join(taker.Config.ArtifactsRoot, rel, "data"), []byte("corrupt"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				dump := os.Getenv("CORPUS_PG_DUMP")
				if dump == "" {
					dump = "pg_dump"
				}
				dump, err = exec.LookPath(dump)
				if err != nil {
					t.Fatal("locate pg_dump:", err)
				}
				detached := false
				taker.Dumper = detachingDumper{dump: PgDump{Path: dump}, detach: func(ctx context.Context) error {
					detached = true
					sql := `DELETE FROM run_item WHERE artifact_id=$1`
					if detach == "reset" {
						sql = `UPDATE run_item SET artifact_id=NULL,state='pending' WHERE artifact_id=$1`
					}
					_, e := db.Exec(ctx, sql, id)
					return e
				}}
				dir, takeErr := taker.Take(ctx, "vn-20990111_1", 1, "fetch")
				if !detached {
					t.Fatalf("dump did not reach detachment seam: %v", takeErr)
				}
				var refs int
				if err = db.QueryRow(ctx, `SELECT count(*) FROM run_item WHERE artifact_id=$1`, id).Scan(&refs); err != nil || refs != 0 {
					t.Fatalf("live references=%d: %v", refs, err)
				}
				marker := filepath.Join(taker.Config.Dir, "vn-20990111_1", "1-fetch", "checkpoint.json")
				if corrupt {
					if takeErr == nil || !strings.Contains(takeErr.Error(), "verify referenced artifact") {
						t.Fatalf("corrupt artifact from dump was not required: %v", takeErr)
					}
					if _, err = os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
						t.Fatalf("failed checkpoint published marker: %v", err)
					}
					return
				}
				if takeErr != nil {
					t.Fatal(takeErr)
				}
				if _, err = os.Stat(filepath.Join(dir, "checkpoint.json")); err != nil {
					t.Fatal(err)
				}
				if got, e := artifact.Verify(filepath.Join(taker.Config.Dir, "artifacts", rel)); e != nil || got != sum {
					t.Fatalf("copied artifact hash=%s: %v", got, e)
				}
			})
		}
	}
}
