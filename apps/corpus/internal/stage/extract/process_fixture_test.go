package extract

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Only Process's two read queries cross this fixture. Source selection,
// parsing, lineage checks and artifact publication all use production code.
func processFixtureDB(t *testing.T, fetch artifact.Result) *pgxpool.Pool {
	t.Helper()
	cfg, err := pgxpool.ParseConfig("host=127.0.0.1 user=fixture dbname=fixture sslmode=disable pool_max_conns=1")
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	var workers sync.WaitGroup
	cfg.ConnConfig.DialFunc = func(context.Context, string, string) (net.Conn, error) {
		client, server := net.Pipe()
		workers.Add(1)
		go func() {
			defer workers.Done()
			defer func() { _ = server.Close() }()
			if serveErr := serveProcessFixture(server, fetch); serveErr != nil && !errors.Is(serveErr, io.EOF) && !errors.Is(serveErr, net.ErrClosed) {
				t.Errorf("process query fixture: %v", serveErr)
			}
		}()
		return client, nil
	}
	db, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); workers.Wait() })
	return db
}

func serveProcessFixture(conn net.Conn, fetch artifact.Result) error {
	backend := pgproto3.NewBackend(conn, conn)
	if _, err := backend.ReceiveStartupMessage(); err != nil {
		return err
	}
	backend.Send(&pgproto3.AuthenticationOk{})
	for key, value := range map[string]string{"server_version": "18.0", "client_encoding": "UTF8", "standard_conforming_strings": "on"} {
		backend.Send(&pgproto3.ParameterStatus{Name: key, Value: value})
	}
	backend.Send(&pgproto3.BackendKeyData{ProcessID: 1, SecretKey: []byte{0, 0, 0, 1}})
	backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
	if err := backend.Flush(); err != nil {
		return err
	}
	for {
		message, err := backend.Receive()
		if err != nil {
			return err
		}
		switch message := message.(type) {
		case *pgproto3.Terminate:
			return nil
		case *pgproto3.Query:
			switch {
			case strings.HasPrefix(message.String, "-- name: GetExtractInput :one"):
				backend.Send(&pgproto3.RowDescription{Fields: []pgproto3.FieldDescription{
					{Name: []byte("path"), DataTypeOID: pgtype.TextOID, DataTypeSize: -1, TypeModifier: -1},
					{Name: []byte("sha256"), DataTypeOID: pgtype.TextOID, DataTypeSize: -1, TypeModifier: -1},
				}})
				backend.Send(&pgproto3.DataRow{Values: [][]byte{[]byte(fetch.Path), []byte(fetch.SHA256)}})
				backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})
			case strings.HasPrefix(message.String, "-- name: GetRunOCRSelection :one"):
				backend.Send(&pgproto3.RowDescription{})
				backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 0")})
			default:
				return fmt.Errorf("unexpected query")
			}
			backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
			if err = backend.Flush(); err != nil {
				return err
			}
		default:
			return fmt.Errorf("unexpected protocol message %T", message)
		}
	}
}
