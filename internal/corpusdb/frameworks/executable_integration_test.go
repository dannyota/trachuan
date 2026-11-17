//go:build integration

package frameworks

import (
	"context"
	"slices"
	"testing"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func TestExecutableSQL_RestoresSameDatabaseObjectsAsBuild(t *testing.T) {
	ctx := t.Context()
	built := schemaDB(t)
	restored, err := pgx.Connect(ctx, testpg.NewDatabase(t, "vector"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = restored.Close(context.Background()) }()
	tables, err := TablesSQL(1024)
	if err != nil {
		t.Fatal(err)
	}
	executable, err := ExecutableSQL(1)
	if err != nil {
		t.Fatal(err)
	}
	for _, sql := range []string{tables, indexes, executable} {
		if _, err = restored.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	objects := func(conn *pgx.Conn) []string {
		t.Helper()
		rows, queryErr := conn.Query(ctx, `SELECT definition FROM (
 SELECT 'view '||c.relname||' '||pg_get_viewdef(c.oid) AS definition FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind='v'
 UNION ALL SELECT 'trigger '||pg_get_triggerdef(t.oid) FROM pg_trigger t JOIN pg_class c ON c.oid=t.tgrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND NOT t.tgisinternal
 UNION ALL SELECT 'function '||pg_get_functiondef(p.oid) FROM pg_proc p JOIN pg_namespace n ON n.oid=p.pronamespace WHERE n.nspname='public' AND p.prorettype='trigger'::regtype
 UNION ALL SELECT 'constraint '||c.relname||' '||con.conname||' '||pg_get_constraintdef(con.oid) FROM pg_constraint con JOIN pg_class c ON c.oid=con.conrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public'
 UNION ALL SELECT 'index '||indexdef FROM pg_indexes WHERE schemaname='public'
 UNION ALL SELECT 'column '||c.relname||' '||a.attname||' '||format_type(a.atttypid,a.atttypmod)||' '||a.attnotnull FROM pg_attribute a JOIN pg_class c ON c.oid=a.attrelid JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind='r' AND a.attnum>0 AND NOT a.attisdropped
 ) objects ORDER BY definition`)
		if queryErr != nil {
			t.Fatal(queryErr)
		}
		defer rows.Close()
		var result []string
		for rows.Next() {
			var definition string
			if scanErr := rows.Scan(&definition); scanErr != nil {
				t.Fatal(scanErr)
			}
			result = append(result, definition)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return result
	}
	if !slices.Equal(objects(built), objects(restored)) {
		t.Fatal("restored objects differ from build")
	}
}
