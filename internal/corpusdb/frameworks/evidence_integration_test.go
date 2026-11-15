//go:build integration

package frameworks

import (
	"context"
	"testing"
)

func TestSchema_EvidenceConstraints(t *testing.T) {
	conn := schemaDB(t)
	seed(t, conn)
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `DELETE FROM mapping_evidence`); err != nil {
		t.Fatal(err)
	}
	cases := map[string]violationCase{
		"dependency omitted":          {`INSERT INTO mapping_evidence(mapping_id,evidence) VALUES(1,'Invented')`, "23514", "mapping_evidence_dependencies"},
		"dependency null":             {`INSERT INTO mapping_evidence VALUES(1,'Invented',NULL)`, "23514", "mapping_evidence_dependencies"},
		"dependency null element":     {`INSERT INTO mapping_evidence VALUES(1,'Invented',ARRAY[NULL]::bigint[])`, "23514", "mapping_evidence_dependencies"},
		"dependency zero":             {`INSERT INTO mapping_evidence VALUES(1,'Invented','{0}')`, "23503", "mapping_evidence_dependencies"},
		"dependency multidimensional": {`INSERT INTO mapping_evidence VALUES(1,'Invented','{{1,2},{3,4}}')`, "23514", "mapping_evidence_dependencies"},
		"dependency unsorted":         {`INSERT INTO mapping_evidence VALUES(1,'Invented','{3,1}')`, "23514", "mapping_evidence_dependencies"},
		"dependency duplicate":        {`INSERT INTO mapping_evidence VALUES(1,'Invented','{1,1}')`, "23514", "mapping_evidence_dependencies"},
		"dependency unknown":          {`INSERT INTO mapping_evidence VALUES(1,'Invented','{99}')`, "23503", "mapping_evidence_dependencies"},
		"dependency free":             {`INSERT INTO mapping_evidence VALUES(1,'Invented','{2}')`, "23503", "mapping_evidence_dependencies"},
		"mapping missing":             {`INSERT INTO mapping_evidence VALUES(99,'Invented','{}')`, "23503", "mapping_evidence_mapping_fkey"},
		"duplicate":                   {`INSERT INTO mapping_evidence VALUES(1,'Invented','{}'),(1,'Invented again','{}')`, "23505", "mapping_evidence_pkey"},
	}
	runViolations(t, conn, cases)
}
