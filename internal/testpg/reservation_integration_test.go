//go:build integration

package testpg

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
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
	results := make(chan string, 2)
	done := make(chan struct{})
	var first, second string
	t.Run("parallel", func(t *testing.T) {
		go func() { first = <-results; second = <-results; close(done) }()
		for _, name := range []string{"a", "b"} {
			t.Run(name, func(t *testing.T) {
				t.Parallel()
				results <- releaseForName(t, "collision")
				<-done
			})
		}
	})
	if first == second {
		t.Fatalf("colliding tests received %q", first)
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
