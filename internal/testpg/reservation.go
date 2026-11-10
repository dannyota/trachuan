//go:build integration

package testpg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"testing"
)

var releases sync.Map

type reservation struct {
	once    sync.Once
	release string
}

// Release reserves a test release until the test and its cleanups finish.
func Release(t *testing.T) string {
	t.Helper()
	return releaseForName(t, t.Name())
}

func releaseForName(t *testing.T, name string) string {
	t.Helper()
	value, _ := releases.LoadOrStore(t, &reservation{})
	entry := value.(*reservation)
	entry.once.Do(func() {
		conn, _ := administrator(t)
		ctx := context.Background()
		for _, rel := range releaseCandidates(name) {
			hash := sha256.Sum256([]byte("trachuan-test-release:" + rel))
			key := int64(binary.BigEndian.Uint64(hash[:8]))
			var locked bool
			if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&locked); err != nil {
				t.Fatalf("reserve test release lock: %v", err)
			}
			if !locked {
				continue
			}
			var exists bool
			if err := conn.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_database WHERE datname=$1)`, "corpus_vn_"+rel).Scan(&exists); err != nil {
				t.Fatalf("inspect test release database: %v", err)
			}
			if exists {
				var unlocked bool
				if err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, key).Scan(&unlocked); err != nil || !unlocked {
					t.Fatalf("unlock existing test release: unlocked=%v error=%v", unlocked, err)
				}
				continue
			}
			entry.release = rel
			t.Cleanup(func() {
				var unlocked bool
				if err := conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, key).Scan(&unlocked); err != nil || !unlocked {
					t.Errorf("unlock test release: unlocked=%v error=%v", unlocked, err)
				}
				releases.Delete(t)
			})
			return
		}
		t.Fatal("no test release available in 2099")
	})
	return entry.release
}
