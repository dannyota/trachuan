package main

import (
	"os"
	"strings"
	"testing"
)

func TestCapacityScript_ActivationWritesOnlyAuthRelease(t *testing.T) {
	data, err := os.ReadFile("../../../../scripts/capacity-local.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	start := strings.Index(script, "cat > \"$tmp_dir/activate\" <<'ACTIVATE'")
	if start < 0 {
		t.Fatal("activation helper missing")
	}
	end := strings.Index(script[start:], "\nACTIVATE\n")
	if end < 0 {
		t.Fatal("activation helper end missing")
	}
	helper := script[start : start+end]
	for _, want := range []string{`UPDATE corpus_release SET`, `WHERE country = 'vn'`, `active_db = :'expected'`, `summary_database = :'target'`, `active_db = summary_database`, `summary_projection = 1`, `headings_approved = true`, `psql -X "$SERVER_AUTH_DATABASE_URL"`, `corpus_vn_20261117_7`, `corpus_vn_20261117_5`} {
		if !strings.Contains(helper, want) {
			t.Errorf("activation missing %q", want)
		}
	}
	for _, bad := range []string{"INSERT ", "DELETE ", "DROP ", "CREATE ", "ALTER ", "TRUNCATE ", "UPDATE document", "UPDATE provision"} {
		if strings.Contains(strings.ToUpper(helper), strings.ToUpper(bad)) {
			t.Errorf("forbidden operation %q", bad)
		}
	}
	if strings.Count(helper, "UPDATE ") != 1 {
		t.Fatal("unexpected update statements")
	}
	if !strings.Contains(script, `default_transaction_read_only`) {
		t.Fatal("corpus connections are not read-only")
	}
}

func TestCapacityScript_CorpusConnectionsReadOnly(t *testing.T) {
	data, err := os.ReadFile("../../../../scripts/capacity-local.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, want := range []string{`params.append(("default_transaction_read_only", "on"))`, `corpus_url="$(database_url '{database}' read-only)"`, `query_url="$(database_url corpus_vn_20261117_7 read-only)"`, `--corpus-db "$query_url"`} {
		if !strings.Contains(script, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, line := range strings.Split(script, "\n") {
		upper := strings.ToUpper(line)
		if strings.Contains(upper, "INSERT INTO ") && !strings.Contains(line, `INSERT INTO corpus_release`) {
			t.Errorf("unexpected insert: %s", line)
		}
		if strings.Contains(upper, "UPDATE ") && !strings.Contains(line, `UPDATE corpus_release SET`) {
			t.Errorf("unexpected update: %s", line)
		}
		if strings.Contains(upper, "DROP DATABASE") && !strings.Contains(line, `DROP DATABASE IF EXISTS :"db" WITH (FORCE)`) {
			t.Errorf("unexpected database drop: %s", line)
		}
		if strings.Contains(upper, "CREATE DATABASE") && !strings.Contains(line, `CREATE DATABASE :"db"`) {
			t.Errorf("unexpected database creation: %s", line)
		}
		if strings.Contains(upper, "DELETE FROM ") || strings.Contains(upper, "TRUNCATE ") || strings.Contains(upper, "ALTER TABLE ") {
			t.Errorf("unexpected write: %s", line)
		}
	}
}

func TestCapacityScript_RequiresRenderEvidence(t *testing.T) {
	data, err := os.ReadFile("../../../../scripts/capacity-local.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "capacity: summary render log missing") {
		t.Fatal("missing render evidence guard")
	}
}
