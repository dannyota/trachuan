package build

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

func databaseMetrics(ctx context.Context, conn *pgx.Conn, loaded Counts) (map[string]int64, int64, int64, error) {
	tables := []string{"document", "source_file", "provision", "provision_validity", "chunk", "lexical_df", "relation", "coverage_gap", "taxonomy_node", "taxonomy_assignment", "release"}
	counts := make(map[string]int64, len(tables)+2)
	for _, table := range tables {
		var n int64
		if err := conn.QueryRow(ctx, "SELECT count(*) FROM "+pgx.Identifier{table}.Sanitize()).Scan(&n); err != nil {
			return nil, 0, 0, fmt.Errorf("count %s: %w", table, err)
		}
		counts[table] = n
	}
	if counts["document"] != loaded.Documents || counts["source_file"] != loaded.SourceFiles || counts["provision"] != loaded.Provisions || counts["chunk"] != loaded.Chunks || counts["relation"] != loaded.Relations || counts["coverage_gap"] != loaded.Gaps || loaded.Chunks != loaded.DenseVectors || counts["taxonomy_node"] != 0 || counts["taxonomy_assignment"] != 0 || counts["release"] != 1 {
		return nil, 0, 0, fmt.Errorf("corpus row counts differ")
	}
	counts["rejected"] = loaded.Rejected
	counts["unavailable_artifacts"] = loaded.UnavailableArtifacts
	var relationBytes, indexBytes int64
	if err := conn.QueryRow(ctx, `SELECT COALESCE(sum(pg_relation_size(c.oid)),0), COALESCE(sum(pg_indexes_size(c.oid)),0) FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN ('r','p')`).Scan(&relationBytes, &indexBytes); err != nil {
		return nil, 0, 0, fmt.Errorf("measure corpus: %w", err)
	}
	return counts, relationBytes, indexBytes, nil
}
