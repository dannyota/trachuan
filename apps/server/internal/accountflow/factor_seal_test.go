package accountflow

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/server/internal/factor"
	"danny.vn/trachuan/apps/server/internal/seal"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type resealTx struct {
	pgx.Tx
	data []byte
	key  string
	err  error
}

func (tx *resealTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if !strings.HasPrefix(sql, "UPDATE totp_") {
		return pgconn.CommandTag{}, errors.New("unexpected mutation")
	}
	tx.data, tx.key = args[1].([]byte), args[2].(string)
	return pgconn.NewCommandTag("UPDATE 1"), tx.err
}
func testRing(t *testing.T, values ...byte) *seal.Ring {
	t.Helper()
	lines := make([]string, len(values))
	for i, v := range values {
		lines[i] = base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{v}, 32))
	}
	ring, err := seal.Parse([]byte(strings.Join(lines, "\n")))
	if err != nil {
		t.Fatal(err)
	}
	return ring
}
func TestFactorSeal_PurposeSeparationAndResealInCallerTransaction(t *testing.T) {
	old, ring := testRing(t, 1), testRing(t, 2, 1)
	for _, purpose := range []string{"totp", "totp_enrollment"} {
		t.Run(purpose, func(t *testing.T) {
			table := "totp_credential"
			if purpose == "totp_enrollment" {
				table = purpose
			}
			binding := seal.Binding{Purpose: purpose, OwnerKind: "account", OwnerID: 12, RowID: 34}
			secret := []byte("12345678901234567890")
			data, key, err := old.Seal(binding, secret)
			if err != nil {
				t.Fatal(err)
			}
			tx := &resealTx{}
			s := Service{Ring: ring}
			plain, err := s.openFactor(t.Context(), tx, table, purpose, 34, 12, key, data)
			if err != nil || !bytes.Equal(plain, secret) || tx.key != ring.ActiveID() {
				t.Fatal("old secret not resealed", err)
			}
			plain, previous, err := ring.Open(binding, tx.key, tx.data)
			if err != nil || previous || !bytes.Equal(plain, secret) {
				t.Fatal("resealed secret invalid", err)
			}
			wrong := binding
			wrong.Purpose = "mail"
			if _, _, err = ring.Open(wrong, key, data); err == nil {
				t.Fatal("purpose separation failed")
			}
			tx = &resealTx{err: errors.New("synthetic write failure")}
			if _, err = s.openFactor(t.Context(), tx, table, purpose, 34, 12, key, data); err == nil || errors.Is(err, factor.ErrFailed) {
				t.Fatal("reseal write failure ignored")
			}
		})
	}
}
func TestFactorSeal_UnknownKeyFailsClosedAndAlarms(t *testing.T) {
	var logs bytes.Buffer
	s := Service{Ring: testRing(t, 2), Logger: slog.New(slog.NewTextHandler(&logs, nil))}
	old := testRing(t, 1)
	data, key, err := old.Seal(seal.Binding{Purpose: "totp", OwnerKind: "account", OwnerID: 12, RowID: 34}, []byte("12345678901234567890"))
	if err != nil {
		t.Fatal(err)
	}
	tx := &resealTx{}
	if plain, err := s.openFactor(t.Context(), tx, "totp_credential", "totp", 34, 12, key, data); !errors.Is(err, factor.ErrFailed) || plain != nil || tx.data != nil {
		t.Fatal("unknown key opened")
	}
	if !strings.Contains(logs.String(), "totp_open_failed") || strings.Contains(logs.String(), "12345678901234567890") {
		t.Fatal("missing or unsafe alarm")
	}
}
