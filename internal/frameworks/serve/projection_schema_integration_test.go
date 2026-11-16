//go:build integration

package serve

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

type schemaReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}

func readRelationClasses(t *testing.T, db schemaReader) map[string]relationClass {
	t.Helper()
	rows, err := db.Query(context.Background(), `SELECT c.relname,c.relkind::text,
 coalesce(array_agg(DISTINCT CASE WHEN refns.nspname='public' THEN ref.relname::text
 ELSE refns.nspname::text||'.'||ref.relname::text END)
 FILTER (WHERE ref.oid IS NOT NULL),'{}'::text[])
 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace
 LEFT JOIN pg_rewrite r ON r.ev_class=c.oid AND c.relkind IN ('v','m')
 LEFT JOIN pg_depend d ON d.classid='pg_rewrite'::regclass AND d.objid=r.oid AND d.refclassid='pg_class'::regclass
 LEFT JOIN pg_class ref ON ref.oid=d.refobjid AND ref.oid<>c.oid
 LEFT JOIN pg_namespace refns ON refns.oid=ref.relnamespace
 WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m')
 GROUP BY c.oid,c.relname,c.relkind ORDER BY c.relname`)
	require.NoError(t, err)
	defer rows.Close()
	var relations []schemaRelation
	for rows.Next() {
		var relation schemaRelation
		require.NoError(t, rows.Scan(&relation.name, &relation.kind, &relation.dependencies))
		relations = append(relations, relation)
	}
	require.NoError(t, rows.Err())
	classes := classifyRelations(relations)
	require.Equal(t, textRelation, classes["body_lexical_df"])
	require.Equal(t, textRelation, classes["body_lexical_stats"])
	require.Equal(t, publicRelation, classes["public_lexical_df"])
	require.Equal(t, publicRelation, classes["public_lexical_stats"])
	return classes
}

func TestProjectionTrace_RejectsDiscardedTextDerivedReads(t *testing.T) {
	e, trace := buildFixture(t, false)
	ctx := context.Background()
	_, err := e.db.Exec(ctx, `CREATE VIEW invented_body_summary AS SELECT version_id,count(*) AS chunks FROM body_chunk GROUP BY version_id`)
	require.NoError(t, err)
	_, err = e.db.Exec(ctx, `CREATE VIEW invented_body_rollup AS SELECT count(*) FROM invented_body_summary`)
	require.NoError(t, err)
	trace.classes = readRelationClasses(t, e.db)
	require.Equal(t, textRelation, trace.classes["invented_body_summary"])
	require.Equal(t, textRelation, trace.classes["invented_body_rollup"])
	var private []string
	for name, class := range trace.classes {
		if class == textRelation {
			private = append(private, name)
		}
	}
	slices.Sort(private)
	caller := fakeCaller()
	request := Request{Query: "quartz"}
	trace.reset()
	baseline, err := e.Search(ctx, request, caller)
	require.NoError(t, err)
	trace.assertProjection(t, e.project(caller).readable)
	for _, name := range private {
		t.Run(name, func(t *testing.T) {
			trace.reset()
			response, searchErr := e.Search(ctx, request, caller)
			require.NoError(t, searchErr)
			require.Equal(t, responseBytes(t, baseline, nil), responseBytes(t, response, nil))
			require.NoError(t, trace.projectionError(e.project(caller).readable))
			_, discardErr := e.db.Exec(ctx, "SELECT count(*) FROM "+pgx.Identifier{"public", name}.Sanitize())
			require.NoError(t, discardErr)
			require.ErrorContains(t, trace.projectionError(e.project(caller).readable), "unclassified statement")
		})
	}
}
