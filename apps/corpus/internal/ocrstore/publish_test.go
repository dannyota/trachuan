package ocrstore

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

type publishDB struct{ events *[]string }

func (d publishDB) Exec(context.Context, string, ...interface{}) (pgconn.CommandTag, error) {
	panic("unexpected exec")
}
func (d publishDB) Query(context.Context, string, ...interface{}) (pgx.Rows, error) {
	panic("unexpected query")
}
func (d publishDB) QueryRow(context.Context, string, ...interface{}) pgx.Row {
	*d.events = append(*d.events, "insert")
	return publishRow{}
}

type publishRow struct{}

func (publishRow) Scan(dest ...interface{}) error { *dest[0].(*int64) = 1; return nil }

func TestStore_PublishSyncsBeforeInsert(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprint("directory failure ", fail), func(t *testing.T) {
			events := []string{}
			s := Open(t.TempDir(), publishDB{events: &events})
			s.syncFile = func(f *os.File) error {
				info, err := f.Stat()
				if err != nil || info.Size() != 4 {
					t.Fatal("text was not written before sync")
				}
				events = append(events, "file sync")
				return f.Sync()
			}
			s.linkFile = func(old, new string) error { events = append(events, "link"); return os.Link(old, new) }
			s.syncDir = func(dir string) error {
				events = append(events, "directory sync")
				if fail {
					return errors.New("sync failure")
				}
				return syncDirectory(dir)
			}
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte("pdf")))
			_, err := s.Put(context.Background(), hash, "engine", "banhmi", pgtype.Int4{}, []byte("text"))
			expected := []string{"file sync", "link", "directory sync"}
			if !fail {
				expected = append(expected, "insert")
			}
			if (err != nil) != fail || !reflect.DeepEqual(events, expected) {
				t.Fatalf("events=%v err=%v", events, err)
			}
		})
	}
}
