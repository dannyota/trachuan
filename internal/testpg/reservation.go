//go:build integration

package testpg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

var releases sync.Map

type reservation struct {
	once     sync.Once
	mu       sync.Mutex
	release  string
	conn     *pgx.Conn
	key      int64
	released bool
	lost     bool
}

// Release reserves a test release until the test and its cleanups finish.
func Release(t *testing.T) string {
	t.Helper()
	return releaseForName(t, t.Name())
}

func releaseForName(t *testing.T, name string) string {
	t.Helper()
	return releaseForNameChecked(t, name, func() { t.Fatal("test release reservation lost") })
}

func releaseForNameChecked(t *testing.T, name string, fail func()) string {
	t.Helper()
	value, _ := releases.LoadOrStore(t, &reservation{})
	entry := value.(*reservation)
	created := false
	entry.once.Do(func() {
		created = true
		entry.conn, _ = administrator(t)
		var err error
		entry.release, entry.key, err = acquireReservation(context.Background(), entry.conn, name)
		if err != nil {
			t.Fatalf("reserve test release: %v", err)
		}
		t.Cleanup(func() {
			entry.mu.Lock()
			defer entry.mu.Unlock()
			if !entry.lost {
				var unlocked bool
				if err := entry.conn.QueryRow(context.Background(), `SELECT pg_advisory_unlock($1)`, entry.key).Scan(&unlocked); err != nil || !unlocked {
					t.Errorf("unlock test release: unlocked=%v error=%v", unlocked, err)
				}
			}
			entry.released = true
		})
	})
	if !created {
		entry.mu.Lock()
		defer entry.mu.Unlock()
		if !entry.released {
			ctx := context.Background()
			var held bool
			err := entry.conn.Ping(ctx)
			if err == nil {
				err = entry.conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_locks WHERE locktype='advisory' AND pid=pg_backend_pid() AND classid::bigint=(($1::bigint >> 32) & 4294967295) AND objid::bigint=($1::bigint & 4294967295) AND objsubid=1 AND mode='ExclusiveLock' AND granted)`, entry.key).Scan(&held)
			}
			if err != nil || !held {
				entry.lost = true
				fail()
				return ""
			}
		}
	}
	return entry.release
}

func acquireReservation(ctx context.Context, conn *pgx.Conn, name string) (string, int64, error) {
	for _, rel := range releaseCandidates(name) {
		hash := sha256.Sum256([]byte("trachuan-test-release:" + rel))
		key := int64(binary.BigEndian.Uint64(hash[:8]))
		var locked bool
		if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&locked); err != nil {
			return "", 0, fmt.Errorf("lock candidate: %w", err)
		}
		if !locked {
			continue
		}
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, "corpus_vn_"+rel).Scan(&exists); err != nil {
			return "", 0, fmt.Errorf("inspect candidate database: %w", err)
		}
		if exists {
			var unlocked bool
			if err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, key).Scan(&unlocked); err != nil {
				return "", 0, fmt.Errorf("unlock existing candidate: %w", err)
			}
			if !unlocked {
				return "", 0, fmt.Errorf("unlock existing candidate: lock not held")
			}
			continue
		}
		return rel, key, nil
	}
	return "", 0, fmt.Errorf("no test release available in 2099")
}
