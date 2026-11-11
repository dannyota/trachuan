package oauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type clientRow struct {
	row store.OauthClient
	err error
}

func (r clientRow) Scan(values ...any) error {
	if r.err != nil {
		return r.err
	}
	*values[0].(*int64) = r.row.ID
	*values[1].(*string) = r.row.ClientID
	*values[2].(*string) = r.row.Kind
	*values[3].(*string) = r.row.Name
	*values[4].(*[]string) = r.row.RedirectUris
	*values[5].(**string) = r.row.MetadataSha256
	*values[6].(*time.Time) = r.row.CreatedAt
	*values[7].(*time.Time) = r.row.LastUsedAt
	return nil
}

type clientDBFake struct {
	store.DBTX
	tx                pgx.Tx
	row               store.OauthClient
	readErr, beginErr error
	begins            int
}

func (d *clientDBFake) Begin(context.Context) (pgx.Tx, error) { d.begins++; return d.tx, d.beginErr }
func (d *clientDBFake) QueryRow(context.Context, string, ...any) pgx.Row {
	return clientRow{d.row, d.readErr}
}

type admissionFailureTx struct {
	pgx.Tx
	commitErr, errorOnExec error
	rollback               bool
}

func (tx *admissionFailureTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, tx.errorOnExec
}
func (tx *admissionFailureTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return clientRow{row: store.OauthClient{ClientID: "fixture", Kind: "dcr", Name: "Fixture", RedirectUris: []string{"http://localhost/cb"}}}
}
func (tx *admissionFailureTx) Commit(context.Context) error   { return tx.commitErr }
func (tx *admissionFailureTx) Rollback(context.Context) error { tx.rollback = true; return nil }

func TestClients_ResolveOrderAndFailures(t *testing.T) {
	ctx := t.Context()
	db := &clientDBFake{row: store.OauthClient{ClientID: "stored", Kind: "dcr", Name: "Stored", RedirectUris: []string{"http://localhost/cb"}}}
	clients := NewClients(db, []Client{{ID: "https://example.com/client", Name: "Must not override CIMD"}}, nil)
	if _, err := clients.Resolve(ctx, "https://example.com/client"); !errors.Is(err, ErrInvalidClient) {
		t.Fatalf("HTTPS bypassed CIMD: %v", err)
	}
	got, err := clients.Resolve(ctx, "stored")
	if err != nil || got.Name != "Stored" || got.Kind != "dcr" {
		t.Fatalf("DCR %v %v", got, err)
	}
	got.RedirectURIs[0] = "https://changed.example/cb"
	if db.row.RedirectUris[0] != "http://localhost/cb" {
		t.Fatal("row aliases resolver result")
	}
	db.readErr = pgx.ErrNoRows
	if _, err := clients.Resolve(ctx, "unknown"); !errors.Is(err, ErrInvalidClient) {
		t.Fatalf("unknown error %v", err)
	}
	db.readErr = errors.New("unavailable")
	if _, err := clients.Resolve(ctx, "stored"); !errors.Is(err, ErrTemporarilyUnavailable) {
		t.Fatalf("DB error %v", err)
	}
	db.beginErr = errors.New("begin failure")
	clients = NewClients(db, []Client{{ID: "configured", Name: "Configured", Kind: "preregistered", RedirectURIs: []string{"http://localhost/cb"}}}, nil)
	if _, err := clients.Resolve(ctx, "configured"); !errors.Is(err, ErrTemporarilyUnavailable) || db.begins != 1 {
		t.Fatalf("configured client did not take admission: %v", err)
	}
}

func TestAdmission_FailuresAndUnknownCommit(t *testing.T) {
	for _, tc := range []struct {
		name               string
		execErr, commitErr error
	}{
		{"lock", errors.New("lock failed"), nil},
		{"unknown commit", nil, errors.New("connection lost after commit")},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &admissionFailureTx{errorOnExec: tc.execErr, commitErr: tc.commitErr}
			clients := NewClients(&clientDBFake{tx: tx}, nil, nil)
			got, err := clients.Register(t.Context(), Client{ID: "fixture", Kind: "dcr"})
			if got.ID != "" || !errors.Is(err, ErrTemporarilyUnavailable) || !tx.rollback {
				t.Fatalf("got %v err %v rollback %v", got, err, tx.rollback)
			}
		})
	}
	tx := &admissionFailureTx{errorOnExec: errors.New("shared lock failed")}
	if _, err := InUse(t.Context(), tx, "fixture"); !errors.Is(err, ErrTemporarilyUnavailable) {
		t.Fatal("shared lock failure hidden")
	}
}

func TestClientMetadata_RejectsMalformedRedirects(t *testing.T) {
	for _, uri := range []string{"https://example.com:/cb", "https://example.com:0/cb", "http://localhost:65536/cb", `https://example.com/\evil`, "https://[example.com]/cb"} {
		if validClientRedirect(uri) {
			t.Errorf("accepted %s", uri)
		}
	}
	if _, err := cimdURL("https://example.com:65536/client"); err == nil {
		t.Fatal("bad CIMD port accepted")
	}
	if _, err := metadataObject([]byte(`{"ignored":{"key":1,"key":2}}`)); err == nil {
		t.Fatal("nested duplicate accepted")
	}
	if _, err := metadataObject([]byte(`{"ignored":"` + strings.Repeat("x", 10) + `"}`)); err != nil {
		t.Fatal(err)
	}
}
