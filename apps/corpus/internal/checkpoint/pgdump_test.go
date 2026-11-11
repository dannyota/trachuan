package checkpoint

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

func TestPgDump_PasswordNotInArguments(t *testing.T) {
	cmd, err := (PgDump{Path: "pg_dump"}).command(context.Background(), "postgres://user:secret@localhost/db", "dump", "unit-snapshot")
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

func TestPgDump_RequiresAndPassesExportedSnapshot(t *testing.T) {
	builder, ok := any(PgDump{Path: "pg_dump"}).(interface {
		command(context.Context, string, string, string) (*exec.Cmd, error)
	})
	if !ok {
		t.Fatal("pg_dump command has no exported snapshot parameter")
	}
	const snapshot = "00000003-0000001B-1"
	cmd, err := builder.command(context.Background(), "postgres://test@localhost/workshop", "workshop.dump", snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(cmd.Args, "--snapshot="+snapshot) {
		t.Fatalf("exported snapshot missing from arguments: %v", cmd.Args)
	}
	if _, err = builder.command(context.Background(), "postgres://test@localhost/workshop", "workshop.dump", ""); err == nil {
		t.Fatal("accepted dump without exported snapshot")
	}
}
