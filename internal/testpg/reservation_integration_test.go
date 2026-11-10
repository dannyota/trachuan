//go:build integration

package testpg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestRelease_StableWithinTest(t *testing.T) {
	first := Release(t)
	if got := Release(t); got != first {
		t.Fatalf("release changed from %q to %q", first, got)
	}
}

func TestRelease_DistinctForCollidingNames(t *testing.T) {
	connections := make([]*pgx.Conn, 2)
	for i := range connections {
		connections[i], _ = administrator(t)
	}
	results := make([]string, 2)
	errors := make([]error, 2)
	var wg sync.WaitGroup
	for i, conn := range connections {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i], _, errors[i] = acquireReservation(context.Background(), conn, "collision")
		}()
	}
	wg.Wait()
	for _, err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	if results[0] == results[1] {
		t.Fatalf("colliding tests received %q", results[0])
	}
}

func TestRelease_CleanupKeepsIdentity(t *testing.T) {
	admin, _ := administrator(t)
	var first, later string
	t.Run("reserve", func(t *testing.T) {
		t.Cleanup(func() {
			hash := sha256.Sum256([]byte("trachuan-test-release:" + first))
			key := int64(binary.BigEndian.Uint64(hash[:8]))
			var locked bool
			if err := admin.QueryRow(context.Background(), `SELECT pg_try_advisory_lock($1)`, key).Scan(&locked); err != nil || !locked {
				t.Fatalf("lock released candidate: locked=%v error=%v", locked, err)
			}
			later = Release(t)
		})
		first = Release(t)
	})
	if later != first {
		t.Fatalf("release changed during cleanup from %q to %q", first, later)
	}
}

func TestRelease_LostReservationFails(t *testing.T) {
	Release(t)
	value, _ := releases.Load(t)
	entry := value.(*reservation)
	admin, _ := administrator(t)
	var terminated bool
	if err := admin.QueryRow(context.Background(), `SELECT pg_terminate_backend($1)`, entry.conn.PgConn().PID()).Scan(&terminated); err != nil || !terminated {
		t.Fatalf("terminate reservation: terminated=%v error=%v", terminated, err)
	}
	failed := false
	releaseForNameChecked(t, t.Name(), func() { failed = true })
	if !failed {
		t.Fatal("lost reservation was accepted")
	}
}

func TestRelease_SkipsExistingDatabase(t *testing.T) {
	admin, _ := administrator(t)
	ctx := context.Background()
	seed := t.Name()
	var rel string
	created := false
	t.Run("create", func(t *testing.T) {
		rel = releaseForName(t, seed)
		if rel != releaseCandidates(seed)[0] {
			t.Skip("first candidate is unavailable")
		}
		name := "corpus_vn_" + rel
		if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			t.Fatal(err)
		}
		marker := `{"run_key":"vn/` + rel + `"}`
		if _, err := admin.Exec(ctx, "COMMENT ON DATABASE "+pgx.Identifier{name}.Sanitize()+" IS '"+marker+"'"); err != nil {
			t.Fatal(err)
		}
		created = true
	})
	if !created {
		t.Skip("could not prepare a marked database")
	}
	t.Cleanup(func() { DropCorpusDatabase(t, admin, "corpus_vn_"+rel, "vn/"+rel) })
	t.Run("reserve", func(t *testing.T) {
		if got := releaseForName(t, seed); got == rel {
			t.Fatalf("selected existing database %q", got)
		}
	})
}

func TestRelease_CleanupReleasesLock(t *testing.T) {
	var rel string
	t.Run("reserve", func(t *testing.T) { rel = Release(t) })
	admin, _ := administrator(t)
	hash := sha256.Sum256([]byte("trachuan-test-release:" + rel))
	key := int64(binary.BigEndian.Uint64(hash[:8]))
	var locked bool
	if err := admin.QueryRow(context.Background(), `SELECT pg_try_advisory_lock($1)`, key).Scan(&locked); err != nil || !locked {
		t.Fatalf("lock after cleanup: locked=%v error=%v", locked, err)
	}
}
