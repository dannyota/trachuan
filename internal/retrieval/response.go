package retrieval

import (
	"encoding/json"
	"fmt"
	"time"
)

// Fragment points at a slice of Hit.Text that quotes source text of one
// provision; Complete is true only when it holds every covered source interval
// of that provision.
type Fragment struct {
	Identity  Identity `json:"identity"`
	Start     int      `json:"start"`
	End       int      `json:"end"`
	TextStart int      `json:"text_start"`
	TextEnd   int      `json:"text_end"`
	Role      string   `json:"role"`
	Complete  bool     `json:"complete"`
}

// ValidityDecision records one applied validity rule and its evidence.
type ValidityDecision struct {
	Ordinal        int             `json:"ordinal"`
	Layer          string          `json:"layer"`
	RuleVersion    string          `json:"rule_version"`
	Before         json.RawMessage `json:"before"`
	After          json.RawMessage `json:"after"`
	ActorKey       *string         `json:"actor_key"`
	EffectDate     *Date           `json:"effect_date"`
	RelationID     *int64          `json:"relation_id"`
	Anchor         *string         `json:"anchor"`
	Quote          *string         `json:"quote"`
	ArtifactSHA256 *string         `json:"artifact_sha256"`
}

// Validity is the stamp a hit carries about its document's legal effect.
type Validity struct {
	Decisions     []ValidityDecision `json:"decisions"`
	Unresolved    bool               `json:"unresolved"`
	StatusClass   string             `json:"status_class"`
	StatusBasis   string             `json:"status_basis"`
	Current       bool               `json:"current"`
	AsOf          *Date              `json:"as_of,omitempty"`
	EffectiveFrom *Date              `json:"effective_from,omitempty"`
	EffectiveTo   *Date              `json:"effective_to,omitempty"`
	Basis         string             `json:"basis"`
}

// Relation is one source relation incident to a hit, in source direction.
type Relation struct {
	SourceKey      string  `json:"source_key"`
	TargetKey      *string `json:"target_key,omitempty"`
	SourceNumber   string  `json:"source_number"`
	TargetNumber   string  `json:"target_number"`
	SourceAnchor   *string `json:"source_anchor,omitempty"`
	TargetAnchor   *string `json:"target_anchor,omitempty"`
	Kind           string  `json:"kind"`
	RawLabel       string  `json:"raw_label"`
	Evidence       string  `json:"evidence"`
	EvidenceSHA256 string  `json:"evidence_sha256"`
	Complete       bool    `json:"complete"`
	Method         string  `json:"method"`
	SourceResolved bool    `json:"source_resolved"`
	TargetResolved bool    `json:"target_resolved"`
}

// Gap is a coverage or validity gap reported with a response or a hit.
type Gap struct {
	Kind        string `json:"kind"`
	DocumentKey string `json:"document_key,omitempty"`
	Anchor      string `json:"anchor,omitempty"`
	Message     string `json:"message"`
}

// PathNode identifies one provision in a hit's ancestry.
type PathNode struct {
	Identity Identity `json:"identity"`
	Kind     string   `json:"kind"`
	Label    string   `json:"label"`
	Heading  string   `json:"heading,omitempty"`
}

// Hit is one provision returned by search.
type Hit struct {
	Identity      Identity   `json:"identity"`
	Path          []PathNode `json:"path"`
	BestMember    Identity   `json:"best_member"`
	BestTextStart int        `json:"best_text_start"`
	Number        string     `json:"number"`
	Class         string     `json:"class"`
	Source        string     `json:"source"`
	Citation      string     `json:"citation"`
	Title         string     `json:"title"`
	Text          string     `json:"text"`
	Fragments     []Fragment `json:"fragments"`
	Complete      bool       `json:"complete"`
	NextPart      *int       `json:"next_part,omitempty"`
	SourceURL     *string    `json:"source_url,omitempty"`
	Score         float64    `json:"score"`
	BestCosine    float64    `json:"best_cosine"`
	Stamp         Validity   `json:"stamp"`
	Relations     []Relation `json:"relations"`
	Gaps          []Gap      `json:"gaps"`
}

// Diagnostics describes how a response was produced; it never holds query text.
type Diagnostics struct {
	DenseCount      int              `json:"dense_count"`
	LexicalCount    int              `json:"lexical_count"`
	EligibleCount   int              `json:"eligible_count"`
	RawANNCount     int              `json:"raw_ann_count"`
	Fallback        bool             `json:"fallback"`
	TimingsMS       map[string]int64 `json:"timings_ms"`
	QueryRecipeHash string           `json:"query_recipe_hash"`
}

// Response is the result of one search: primary hits first, then non-current
// hits when requested.
type Response struct {
	Release     string      `json:"release"`
	Hits        []Hit       `json:"hits"`
	NonCurrent  []Hit       `json:"non_current"`
	Abstain     bool        `json:"abstain"`
	Reason      string      `json:"reason,omitempty"`
	Message     string      `json:"message,omitempty"`
	Gaps        []Gap       `json:"gaps"`
	Diagnostics Diagnostics `json:"diagnostics"`
}

// String formats the date as YYYY-MM-DD.
func (d Date) String() string {
	return time.Date(d.Year, d.Month, d.Day, 0, 0, 0, 0, time.UTC).Format(time.DateOnly)
}

// MarshalJSON encodes the date as "YYYY-MM-DD".
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

// UnmarshalJSON accepts only a valid "YYYY-MM-DD" calendar date.
func (d *Date) UnmarshalJSON(b []byte) error {
	var s string
	if err := json.Unmarshal(b, &s); err != nil {
		return fmt.Errorf("decode date: %w", err)
	}
	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return fmt.Errorf("invalid date %q: %w", s, err)
	}
	*d = Date{Year: t.Year(), Month: t.Month(), Day: t.Day()}
	return nil
}
