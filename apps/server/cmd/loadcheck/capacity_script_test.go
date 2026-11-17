package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
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

func TestCapacityScript_RecordsHostNodeOptions(t *testing.T) {
	outputDir := t.TempDir()
	command := exec.Command("bash", "../../../../scripts/capacity-local.sh", "--out", outputDir)
	command.Env = []string{"PATH=/usr/bin:/bin", "E2E_SERVER_PORT=0"}
	if err := command.Run(); err == nil {
		t.Fatal("invalid port accepted")
	}
	recorded, err := os.ReadFile(filepath.Join(outputDir, "web-node-options.txt"))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := exec.Command("sed", "-n", "s/^NODE_OPTIONS=//p", "../../../../deploy/host/web.env").Output()
	if err != nil {
		t.Fatal(err)
	}
	if string(recorded) != string(expected) || len(recorded) == 0 {
		t.Fatal("recorded Node options differ from host")
	}
	data, err := os.ReadFile("../../../../scripts/capacity-local.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `NODE_OPTIONS="$node_options"`) {
		t.Fatal("web does not receive host Node options")
	}
}

func TestCapacityScript_PostgresPatternMatchesWorkers(t *testing.T) {
	data, err := os.ReadFile("../../../../scripts/capacity-local.sh")
	if err != nil {
		t.Fatal(err)
	}
	match := regexp.MustCompile(`--proc 'postgres=([^']+)'`).FindStringSubmatch(string(data))
	if len(match) != 2 {
		t.Fatal("postgres pattern missing")
	}
	pattern, err := regexp.Compile(match[1])
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"/usr/bin/postgres", "postgres:", "/usr/bin/postgres:"} {
		if !pattern.MatchString(name) {
			t.Errorf("does not match %q", name)
		}
	}
	if pattern.MatchString("postgresql-client") {
		t.Fatal("matches unrelated process")
	}
}

func TestCapacityScript_ExplicitActivationDirection(t *testing.T) {
	data, err := os.ReadFile("../../../../scripts/capacity-local.sh")
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	for _, part := range []string{`case "${1:-}" in`, `forward)`, `restore)`, `expected='corpus_vn_20261117_7'`, `target='corpus_vn_20261117_7'`, `active_db = :'target'`} {
		if !strings.Contains(script, part) {
			t.Errorf("missing %s", part)
		}
	}
}

func TestCapacityScript_TracksTrafficGenerator(t *testing.T) {
	data, err := os.ReadFile("../../../../scripts/capacity-local.sh")
	if err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{`ln -s "$tmp_dir/loadcheck" "$tmp_dir/load-gen"`, `--proc 'generator=(^|/)load-gen$'`, `--proc 'loadcheck_both=(^|/)(loadcheck|load-gen)$'`, `"$tmp_dir/load-gen" "${run_args[@]}"`} {
		if !strings.Contains(string(data), part) {
			t.Errorf("missing %s", part)
		}
	}
}
