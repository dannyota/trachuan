package retrieval

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"danny.vn/trachuan/internal/embedding"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/puddle/v2"
)

type failingEmbedder struct {
	fakeEmbedder
	err error
}

func (f *failingEmbedder) Embed(context.Context, string) (embedding.Vector, error) {
	return embedding.Vector{}, f.err
}

func TestPrepare_ClassifiesEmbedderWithoutLeakingCause(t *testing.T) {
	for _, tt := range []struct {
		name        string
		cause, want error
	}{
		{"http429", &embedding.Error{Status: 429, Code: "private"}, ErrEmbedderBusy},
		{"busy", &embedding.Error{Status: 503, Code: "busy"}, ErrEmbedderBusy},
		{"inference", &embedding.Error{Status: 500, Code: "private"}, ErrEmbedder},
		{"transport", errors.New("private query token vector"), ErrEmbedder},
	} {
		t.Run(tt.name, func(t *testing.T) {
			e := &Engine{embedder: &failingEmbedder{err: fmt.Errorf("wrapped: %w", tt.cause)}}
			_, err := e.Prepare(context.Background(), Request{Query: "ngân hàng"})
			if !errors.Is(err, tt.want) || err.Error() != "query embedding failed" {
				t.Fatalf("got %v, want fixed classified failure", err)
			}
		})
	}
}

type failingTx struct {
	pgx.Tx
	err error
}

func (f failingTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, f.err }
func (f failingTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, f.err
}

func TestDatabaseFailures_ClassifiedAndSanitized(t *testing.T) {
	for _, tt := range []struct {
		name        string
		cause, want error
	}{
		{"missing", &pgconn.PgError{Code: "3D000", Message: "private"}, ErrReleaseGone},
		{"shutdown", &pgconn.PgError{Code: "57P01", Message: "private"}, ErrReleaseGone},
		{"dropped", &pgconn.PgError{Code: "57P04", Message: "private"}, ErrReleaseGone},
		{"closed", puddle.ErrClosedPool, ErrReleaseGone},
		{"other", &pgconn.PgError{Code: "08006", Message: "private"}, ErrUnavailable},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tx := failingTx{err: fmt.Errorf("private: %w", tt.cause)}
			for _, call := range []struct {
				name, message string
				run           func() error
			}{
				{"dense", "dense query failed", func() error { _, err := denseQuery(context.Background(), tx, "private"); return err }},
				{"lexical", "lexical query failed", func() error { _, err := lexicalQuery(context.Background(), tx); return err }},
				{"settings", "set retrieval transaction option: database setting failed", func() error { return setLocal(context.Background(), tx, "key", "private") }},
				{"hydrate", "hydrate candidates failed", func() error {
					return hydrate(context.Background(), tx, &ArmGroup{Dense: []Candidate{{ChunkID: 1}}}, "private")
				}},
				{"trees", "hydrate descendant provisions failed", func() error { _, _, err := hydrateTrees(context.Background(), tx, []int64{1}); return err }},
				{"nodes", "query read provisions", func() error { _, err := readNodes(context.Background(), tx, 1, "private"); return err }},
			} {
				t.Run(call.name, func(t *testing.T) {
					err := call.run()
					if !errors.Is(err, tt.want) {
						t.Fatalf("unclassified: %v", err)
					}
					if err.Error() != call.message {
						t.Fatalf("changed message %q", err)
					}
				})
			}
		})
	}
}

func TestEngine_ClosedPoolClassified(t *testing.T) {
	// Construction and Close never dial. Every call fails on acquisition.
	pool, err := pgxpool.New(context.Background(), "postgres://localhost/unused")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	e := &Engine{db: pool, options: Options{Profile: "serving"}, release: Release{Rel: "20991111"}}
	for name, call := range map[string]func() error{
		"read": func() error {
			_, err := e.Read(context.Background(), ReadRequest{Release: "20991111", Items: []ReadItem{{Handle: "vn:test"}}})
			return err
		},
		"status": func() error { _, err := e.Status(context.Background()); return err },
		"gaps":   func() error { _, _, err := e.Gaps(context.Background(), "", 0, 0); return err },
	} {
		t.Run(name, func(t *testing.T) {
			if err := call(); !errors.Is(err, ErrReleaseGone) {
				t.Fatalf("got %v", err)
			}
		})
	}
}

type failingRows struct {
	pgx.Rows
	scan bool
	err  error
}

func (r *failingRows) Next() bool        { return r.scan }
func (r *failingRows) Scan(...any) error { return r.err }
func (r *failingRows) Err() error {
	if r.scan {
		return nil
	}
	return r.err
}
func (r *failingRows) Close() {}

type rowsTx struct {
	pgx.Tx
	rows pgx.Rows
}

func (tx rowsTx) Query(context.Context, string, ...any) (pgx.Rows, error) { return tx.rows, nil }

func TestDatabaseRows_ClassifiedAndSanitized(t *testing.T) {
	for _, scan := range []bool{false, true} {
		for _, gone := range []bool{false, true} {
			want := ErrUnavailable
			cause := errors.New("private query SQL arguments vector")
			if gone {
				cause = &pgconn.PgError{Code: "57P01", Message: "private"}
				want = ErrReleaseGone
			}
			for name, call := range map[string]func(pgx.Tx) error{
				"dense":   func(tx pgx.Tx) error { _, err := denseQuery(context.Background(), tx, ""); return err },
				"lexical": func(tx pgx.Tx) error { _, err := lexicalQuery(context.Background(), tx); return err },
				"hydrate": func(tx pgx.Tx) error {
					return hydrate(context.Background(), tx, &ArmGroup{Dense: []Candidate{{ChunkID: 1}}}, "")
				},
				"trees": func(tx pgx.Tx) error { _, _, err := hydrateTrees(context.Background(), tx, []int64{1}); return err },
				"nodes": func(tx pgx.Tx) error { _, err := readNodes(context.Background(), tx, 1, ""); return err },
				"relations": func(tx pgx.Tx) error {
					_, err := readRelations(context.Background(), tx, "", nil, false, 0, 10)
					return err
				},
				"number": func(tx pgx.Tx) error {
					_, _, _, _, _, err := (&Engine{}).readDocument(context.Background(), tx, ReadItem{Number: "1/2099/QH1"})
					return err
				},
			} {
				t.Run(fmt.Sprintf("%s/scan=%t/gone=%t", name, scan, gone), func(t *testing.T) {
					err := call(rowsTx{rows: &failingRows{scan: scan, err: cause}})
					if !errors.Is(err, want) || strings.Contains(err.Error(), "private") {
						t.Fatalf("unsafe or unclassified %v", err)
					}
				})
			}
		}
	}
}

func TestRead_NextFromNonActiveReleaseFailsBeforeDatabase(t *testing.T) {
	e := &Engine{release: Release{Rel: "20991111"}}
	token := encodeNext(cursor{V: 1, Release: "20991110", Item: ReadItem{Handle: "vn:test#a"}})
	result, err := e.Read(context.Background(), ReadRequest{Release: "20991111", Items: []ReadItem{{Next: token}}})
	if err != nil || result.Release != "20991111" || len(result.Items) != 1 || result.Items[0].Error != "release_expired" {
		t.Fatalf("expired continuation: %+v %v", result, err)
	}
}

func TestSearchMetadata_ClosedPoolClassifiedAndSanitized(t *testing.T) {
	// The pool is closed before use, so these calls never dial PostgreSQL.
	pool, err := pgxpool.New(context.Background(), "postgres://localhost/unused")
	if err != nil {
		t.Fatal(err)
	}
	pool.Close()
	e := &Engine{db: pool, options: Options{Profile: "serving"}}
	hit := Hit{Identity: Identity{DocumentKey: "vn:test"}}
	for _, tt := range []struct {
		name, message string
		call          func() error
	}{
		{"documents", "begin documents transaction", func() error { _, err := e.Documents(context.Background(), []string{"vn:test"}); return err }},
		{"presentation", "load presentation documents: begin documents transaction", func() error {
			_, err := e.Present(context.Background(), Response{Hits: []Hit{hit}}, DetailCompact)
			return err
		}},
		{"decisions", "query validity decisions failed", func() error { return e.LoadDecisions(context.Background(), []Hit{hit}) }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.call()
			if !errors.Is(err, ErrReleaseGone) || err.Error() != tt.message {
				t.Fatalf("got %v, want fixed release-gone failure", err)
			}
		})
	}
}
