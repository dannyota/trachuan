// Package stage defines shared pipeline stage types.
package stage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
)

// Snapshot holds the frozen inputs for a run.
type Snapshot struct {
	SourceList                                       json.RawMessage
	SourceListHash                                   string
	Stages                                           []string
	Recipes                                          map[string]string
	TaxonomyVersion, WorkerVersion, PreviousManifest string
}

// Run identifies a pipeline run and its frozen snapshot.
type Run struct {
	ID           int64
	Country, Rel string
	Generation   int
	Snapshot     Snapshot
}

// Gap describes an incomplete item or missing evidence.
type Gap struct{ Kind, Detail string }

// Item identifies stage work and an optional reason to skip it.
type Item struct {
	Key, InputHash string
	Skip           *Gap
}

// Counts summarizes stage progress and outstanding gaps.
type Counts struct {
	Expected, Done, Gap, Failed, Pending int
	UnacceptedGaps                       map[string]int
}

// RecipeHash hashes the NUL-separated recipe components.
func RecipeHash(stage, version string, extra ...string) string {
	parts := append([]string{stage, version}, extra...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
