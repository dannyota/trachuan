//go:build integration

package frameworks

import (
	"context"
	"net/url"
	"testing"

	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5"
)

func TestCheckLogSettings_DatabaseAndRoleInDatabaseOverrideRole(t *testing.T) {
	ctx := t.Context()
	raw := testpg.NewDatabase(t)
	admin, err := pgx.Connect(ctx, raw)
	if err != nil {
		t.Fatal("connect invented database")
	}
	defer func() { _ = admin.Close(context.Background()) }()
	builder := testpg.NewBuilder(t)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal("parse invented URL")
	}
	name := u.Path[1:]
	config, err := pgx.ParseConfig(builder.URL)
	if err != nil {
		t.Fatal("parse invented reader URL")
	}
	config.Database = name
	check := func(want bool) {
		t.Helper()
		conn, connectErr := pgx.ConnectConfig(ctx, config.Copy())
		if connectErr != nil {
			t.Fatal("connect invented reader")
		}
		defer func() { _ = conn.Close(context.Background()) }()
		if settingsErr := CheckLogSettings(ctx, conn); (settingsErr == nil) != want {
			t.Fatalf("settings accepted=%t, want=%t", settingsErr == nil, want)
		}
	}
	check(false)
	settings := []struct{ name, value string }{{"log_min_messages", "panic"}, {"log_min_error_statement", "panic"}, {"log_min_duration_statement", "-1"}, {"log_statement", "none"}}
	for _, s := range settings {
		if _, err = admin.Exec(ctx, "ALTER DATABASE "+pgx.Identifier{name}.Sanitize()+" SET "+s.name+" = '"+s.value+"'"); err != nil {
			t.Fatal(err)
		}
	}
	check(true)
	role := pgx.Identifier{builder.Role}.Sanitize()
	if _, err = admin.Exec(ctx, "ALTER ROLE "+role+" SET log_statement = 'all'"); err != nil {
		t.Fatal(err)
	}
	check(false)
	for _, s := range settings {
		if _, err = admin.Exec(ctx, "ALTER ROLE "+role+" IN DATABASE "+pgx.Identifier{name}.Sanitize()+" SET "+s.name+" = '"+s.value+"'"); err != nil {
			t.Fatal(err)
		}
	}
	check(true)
}
