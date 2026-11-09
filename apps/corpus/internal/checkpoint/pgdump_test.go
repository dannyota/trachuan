package checkpoint

import (
	"context"
	"strings"
	"testing"
)

func TestPgDump_PasswordNotInArguments(t *testing.T) {
	cmd, err := (PgDump{Path: "pg_dump"}).command(context.Background(), "postgres://user:secret@localhost/db", "dump")
	if err != nil {
		t.Fatal(err)
	}
	for _, arg := range cmd.Args {
		if strings.Contains(arg, "secret") {
			t.Fatalf("password in arguments: %v", cmd.Args)
		}
	}
	found := false
	for _, value := range cmd.Env {
		if value == "PGPASSWORD=secret" {
			found = true
		}
	}
	if !found {
		t.Fatal("PGPASSWORD missing")
	}
}
