package retrieval

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type settingsTx struct {
	pgx.Tx
	values map[string]string
}

func (tx *settingsTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if sql != `SELECT set_config($1,$2,true)` || len(args) != 2 {
		return pgconn.CommandTag{}, fmt.Errorf("expected transaction-local setting")
	}
	key, keyOK := args[0].(string)
	value, valueOK := args[1].(string)
	if !keyOK || !valueOK {
		return pgconn.CommandTag{}, fmt.Errorf("invalid setting")
	}
	tx.values[key] = value
	return pgconn.CommandTag{}, nil
}

func TestExactSettings_TransactionLocalAndReenabled(t *testing.T) {
	tx := &settingsTx{values: map[string]string{}}
	for _, step := range []struct {
		want  string
		apply func(context.Context, pgx.Tx) error
	}{{"off", exactSettings}, {"on", indexSettings}} {
		if err := step.apply(context.Background(), tx); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"enable_indexscan", "enable_bitmapscan"} {
			if tx.values[key] != step.want {
				t.Fatalf("%s = %q, want %q", key, tx.values[key], step.want)
			}
		}
	}
}
