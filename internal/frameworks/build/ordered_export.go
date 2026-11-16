package build

import (
	"bytes"
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
)

func OrderedExport(ctx context.Context, conn *pgx.Conn) ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	for i, table := range []struct{ name, order string }{
		{"framework", "id"}, {"framework_version", "id"}, {"control", "id"},
		{"control_text", "control_id"}, {"source_file", "id"}, {"body_chunk", "id"}, {"public_chunk", "id"},
		{"body_posting", "version_id,chunk_id,token"}, {"public_posting", "version_id,chunk_id,token"},
		{"control_mapping", "id"}, {"mapping_evidence", "mapping_id"},
		{"quality_gap", "kind,version_id,subject,detail"}, {"release", "singleton"},
	} {
		query := `SELECT to_jsonb(r)::text FROM ` + table.name + ` r ORDER BY ` + table.order
		if table.name == "release" {
			query = `SELECT to_jsonb(r)::text FROM (
 SELECT singleton,kind,rel,schema_version,dense_recipe,chunk_recipe,lexical_recipe,retrieval_recipe,
 snapshot_recipe,parse_recipe,relate_recipe,embed_recipe,
 snapshot_input_hash,parse_input_hash,relate_input_hash,chunk_input_hash,embed_input_hash,
 framework_count,version_count,control_count,versions_sealed FROM release
 ) r ORDER BY singleton`
		}
		rows, err := conn.Query(ctx, query)
		if err != nil {
			return nil, hide("query ordered export", err)
		}
		if i > 0 {
			out.WriteByte(',')
		}
		out.WriteString(strconv.Quote(table.name))
		out.WriteString(":[")
		first := true
		for rows.Next() {
			var value string
			if scanErr := rows.Scan(&value); scanErr != nil {
				rows.Close()
				return nil, hide("read ordered export", scanErr)
			}
			if !first {
				out.WriteByte(',')
			}
			out.WriteString(value)
			first = false
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, hide("read ordered export", err)
		}
		out.WriteByte(']')
	}
	out.WriteByte('}')
	return out.Bytes(), nil
}
