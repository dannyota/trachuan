//go:build integration

package build

import (
	"context"
	"testing"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestDatabase_GoGapKindsInsertWithTheirClasses(t *testing.T) {
	ctx := context.Background()
	conn := frameworkDB(t)
	require.NoError(t, schema.ApplyTables(ctx, conn, 1024))
	steps := map[string]stepProvenance{}
	for _, step := range []string{"snapshot", "parse", "relate", "chunk", "embed"} {
		steps[step] = stepProvenance{Recipe: stepRecipe(step), InputHash: stepInputHash(nil)}
	}
	require.NoError(t, initializeRelease(ctx, conn, "20990101", steps))
	//nolint:misspell // Match the registry field names.
	_, err := conn.Exec(ctx, `INSERT INTO framework(id,code,name,publisher,aliases) VALUES(1,'fw-alpha','Invented','Invented','{}');
 INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal) VALUES
 (1,'fw-alpha','v1','en','licensed','synthetic','2099-01-01',1),
 (2,'fw-alpha','v2','en','free','public-domain','2099-01-02',2);
 UPDATE release SET versions_sealed=true;`)
	require.NoError(t, err)
	for kind, rule := range gapKinds() {
		t.Run(kind, func(t *testing.T) {
			for i, access := range []string{"licensed", "free"} {
				class, blocks, classifyErr := classifyGap(kind, access)
				require.NoError(t, classifyErr)
				require.Equal(t, rule.Class, class)
				_, insertErr := conn.Exec(ctx, `INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks) VALUES($1,$2,$3,'X.1','1',$4)`, kind, class, i+1, blocks)
				require.NoError(t, insertErr)
			}
		})
	}
	_, err = conn.Exec(ctx, `INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks) VALUES('unknown','public',1,'X.1','1',false)`)
	var violation *pgconn.PgError
	require.ErrorAs(t, err, &violation)
	require.Equal(t, "23514", violation.Code)
	require.Equal(t, "quality_gap_class_check", violation.ConstraintName)
}
