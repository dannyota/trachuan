//go:build integration

package frameworks

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func insertVersion(t *testing.T, db DB) {
	t.Helper()
	//nolint:misspell // Match the registry field names.
	_, err := db.Exec(context.Background(), `INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal)
 VALUES(3,'fw-alpha','3.0','en','free','synthetic','2099-01-02',2)`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSchema_VersionsAreImmutable(t *testing.T) {
	conn := schemaDB(t)
	seedUnsealed(t, conn)
	insertVersion(t, conn)
	ctx := context.Background()
	//nolint:misspell // Match the registry field names.
	queries := map[string]string{
		"free to licensed": `UPDATE framework_version SET access='licensed' WHERE id=3`,
		"licensed to free": `UPDATE framework_version SET access='free' WHERE id=1`,
		"id":               `UPDATE framework_version SET id=4 WHERE id=3`,
		"rename":           `UPDATE framework_version SET version='4.0' WHERE id=3`,
		"framework":        `UPDATE framework_version SET framework_code='fw-beta' WHERE id=3`,
		"language":         `UPDATE framework_version SET language='vi' WHERE id=3`,
		"unchanged":        `UPDATE framework_version SET access=access WHERE id=3`,
		"delete":           `DELETE FROM framework_version WHERE id=3`,
		"delete and reinsert": `DELETE FROM framework_version WHERE id=3;
   INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal)
   VALUES(3,'fw-alpha','3.0','en','licensed','synthetic','2099-01-02',2)`,
		"rename and reinsert": `UPDATE framework_version SET version='4.0' WHERE id=3;
   INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal)
   VALUES(4,'fw-alpha','3.0','en','licensed','synthetic','2099-01-03',3)`,
	}
	for _, phase := range []string{"unsealed", "sealed"} {
		if phase == "sealed" {
			if _, execErr := conn.Exec(ctx, `UPDATE release SET version_count=3,versions_sealed=true;
 INSERT INTO mapping_evidence VALUES(1,'Invented evidence quoting version 3','{}')`); execErr != nil {
				t.Fatal(execErr)
			}
		}
		for name, query := range queries {
			t.Run(phase+"/"+name, func(t *testing.T) {
				_, err := conn.Exec(ctx, query)
				requireViolation(t, err, "23514", "framework_version_immutable")
			})
		}
	}
}

func TestSchema_VersionsSealIsOneWay(t *testing.T) {
	conn := schemaDB(t)
	seedUnsealed(t, conn)
	ctx := context.Background()
	var sealed bool
	if err := conn.QueryRow(ctx, `SELECT versions_sealed FROM release`).Scan(&sealed); err != nil || sealed {
		t.Fatalf("initial seal=%v error=%v", sealed, err)
	}
	_, err := conn.Exec(ctx, `INSERT INTO mapping_evidence VALUES(1,'Invented public evidence','{}')`)
	requireViolation(t, err, "23514", "mapping_evidence_insert")
	if _, execErr := conn.Exec(ctx, `UPDATE release SET versions_sealed=true`); execErr != nil {
		t.Fatal(execErr)
	}
	//nolint:misspell // Match the registry field names.
	_, err = conn.Exec(ctx, `INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal)
 VALUES(3,'fw-alpha','3.0','en','free','synthetic','2099-01-02',2)`)
	requireViolation(t, err, "23514", "framework_version_insert")
	_, err = conn.Exec(ctx, `UPDATE release SET versions_sealed=false`)
	requireViolation(t, err, "23514", "release_versions_sealed")
	if _, execErr := conn.Exec(ctx, `UPDATE release SET versions_sealed=true;
 INSERT INTO mapping_evidence VALUES(1,'Invented public evidence','{}')`); execErr != nil {
		t.Fatal(execErr)
	}
}

func TestSchema_SealWaitsForVersionInsert(t *testing.T) {
	connA := schemaDB(t)
	seedUnsealed(t, connA)
	ctx := context.Background()
	connB, err := pgx.ConnectConfig(ctx, connA.Config().Copy())
	if err != nil {
		t.Fatal("connect second test session")
	}
	t.Cleanup(func() {
		if closeErr := connB.Close(ctx); closeErr != nil {
			t.Error(closeErr)
		}
	})
	txA, err := connA.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if rollbackErr := txA.Rollback(ctx); rollbackErr != nil && rollbackErr != pgx.ErrTxClosed {
			t.Error(rollbackErr)
		}
	})
	insertVersion(t, txA)
	if _, execErr := connB.Exec(ctx, `SET default_transaction_isolation='read committed'`); execErr != nil {
		t.Fatal(execErr)
	}
	_, err = connB.Exec(ctx, `INSERT INTO mapping_evidence VALUES(1,'Invented public evidence','{}')`)
	requireViolation(t, err, "23514", "mapping_evidence_insert")
	if _, execErr := connB.Exec(ctx, `SET lock_timeout='100ms'`); execErr != nil {
		t.Fatal(execErr)
	}
	_, err = connB.Exec(ctx, `UPDATE release SET versions_sealed=true`)
	requireViolation(t, err, "55P03", "")
	if err := txA.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if _, execErr := connB.Exec(ctx, `UPDATE release SET versions_sealed=true;
 INSERT INTO mapping_evidence VALUES(1,'Invented public evidence','{}')`); execErr != nil {
		t.Fatal(execErr)
	}
	var count int
	if err := connB.QueryRow(ctx, `SELECT count(*) FROM framework_version WHERE id=3 AND version='3.0'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("committed version count=%d error=%v", count, err)
	}
}

func TestSchema_VersionAndEvidenceInsertsRequireRelease(t *testing.T) {
	conn := schemaDB(t)
	ctx := context.Background()
	_, err := conn.Exec(ctx, `INSERT INTO framework VALUES(1,'fw-alpha','Framework Alpha','Invented publisher','{}');
 INSERT INTO control_mapping(id,from_code,from_version,from_citation,to_code,to_version,to_citation,relation,source,resolved)
 VALUES(1,'fw-alpha','2.1','A.1','fw-beta','1.0','B.1','related','invented',false);`)
	if err != nil {
		t.Fatal(err)
	}
	//nolint:misspell // Match the registry field names.
	_, err = conn.Exec(ctx, `INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal)
 VALUES(3,'fw-alpha','3.0','en','free','synthetic','2099-01-02',2)`)
	requireViolation(t, err, "23514", "framework_version_insert")
	_, err = conn.Exec(ctx, `INSERT INTO mapping_evidence VALUES(1,'Invented public evidence','{}')`)
	requireViolation(t, err, "23514", "mapping_evidence_insert")
}

func TestSchema_LifecycleCannotBeReset(t *testing.T) {
	conn := schemaDB(t)
	seed(t, conn)
	cases := map[string]violationCase{
		"release delete and reinsert":               {`DELETE FROM release;` + releaseFixtureSQL, "23514", "release_delete"},
		"release truncate":                          {`TRUNCATE release`, "23514", "release_truncate"},
		"version truncate with explicit dependents": {`TRUNCATE framework_version,source_file,control,control_text,body_chunk,public_chunk,body_posting,public_posting,control_mapping,mapping_evidence,quality_gap`, "23514", "framework_version_immutable"},
		"version truncate cascade":                  {`TRUNCATE framework_version CASCADE`, "23514", "framework_version_immutable"},
		"version truncate foreign key precheck":     {`TRUNCATE framework_version`, "0A000", ""},
		"evidence truncate":                         {`TRUNCATE mapping_evidence`, "23514", "mapping_evidence_truncate"},
	}
	runViolations(t, conn, cases)
}
