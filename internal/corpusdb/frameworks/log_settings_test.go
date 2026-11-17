package frameworks

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

type settingsConnection struct {
	values []string
	err    error
}

func (c settingsConnection) QueryRow(context.Context, string, ...any) pgx.Row { return c }
func (c settingsConnection) Scan(dest ...any) error {
	if c.err != nil {
		return c.err
	}
	for i, v := range c.values {
		*dest[i].(*string) = v
	}
	return nil
}

func TestCheckLogSettings_RequiresEveryEffectiveValue(t *testing.T) {
	want := []string{"panic", "panic", "-1", "none"}
	if err := CheckLogSettings(t.Context(), settingsConnection{values: want}); err != nil {
		t.Fatal(err)
	}
	for i, bad := range []string{"error", "error", "0", "all"} {
		values := append([]string(nil), want...)
		values[i] = bad
		if err := CheckLogSettings(t.Context(), settingsConnection{values: values}); err == nil {
			t.Fatalf("accepted setting %d", i)
		}
	}
	err := CheckLogSettings(t.Context(), settingsConnection{err: errors.New("invented protected database detail")})
	if err == nil || err.Error() != "frameworks log settings unavailable" {
		t.Fatal("unsafe settings error")
	}
}
