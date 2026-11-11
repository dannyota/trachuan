// Package retrieval implements the shared corpus retrieval engine.
package retrieval

import (
	"context"
	"time"

	"danny.vn/trachuan/internal/embedding"
)

// Embedder is the query embedding runtime.
type Embedder interface {
	Recipe(context.Context) (embedding.Recipe, error)
	Embed(context.Context, string) (embedding.Vector, error)
}

// Profile selects serving or evaluation deadlines.
type Profile string

// Mode selects exact or approximate dense retrieval.
type Mode string

// ArmMode selects arms used by later ranking.
type ArmMode string

// Date is a civil calendar date.
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// Request describes a retrieval request.
type Request struct {
	Query       string
	Limit       int
	InForceOnly *bool
	AsOf        *Date
	Mode        Mode
	Arms        ArmMode
}

// Prepared holds a validated request and its single query vector.
type Prepared struct {
	Request    Request
	Validation Validation
	Vector     *embedding.Vector
}

// Options supplies the clock and execution profile.
type Options struct {
	Now     func() time.Time
	Profile Profile
}

// Depths specifies the candidate count per arm.
type Depths struct {
	Dense   int
	Lexical int
}

// Identity identifies a provision in a release.
type Identity struct {
	Country     string `json:"country"`
	DocumentKey string `json:"document_key"`
	Anchor      string `json:"anchor"`
}

// Provision holds source text and ancestry for evidence shaping.
type Provision struct {
	ID       int64
	ParentID *int64
	Identity Identity
	NodeID   string
	Kind     string
	Label    string
	Heading  string
	Ordinal  int
	Preorder int
	Text     string
}

// Document holds source metadata for validity and evidence.
type Document struct {
	Number           string
	Class            string
	Source           string
	ValidityConflict bool
	Decisions        []ValidityDecision
	Title            string
	StatusClass      string
	StatusBasis      string
	IsCurrent        bool
	EffectiveFrom    *time.Time
	EffectiveTo      *time.Time
	SourceURL        *string
	DerivationGaps   []string
}

// Candidate is an ordered candidate and its hydrated source metadata.
type Candidate struct {
	ChunkID       int64
	Ordinal       int
	Identity      Identity
	Group         Identity
	Rank          int
	Cosine        float64
	BM25          float64
	ProvisionID   int64
	GroupID       int64
	Content       string
	ContextPrefix string
	Spans         []byte
	Provision     Provision
	Document      Document
	Ancestors     []Provision
	Descendants   []Provision
	Relations     []SourceRelation
}

// SourceRelation associates a source edge with its stable artifact identity.
type SourceRelation struct {
	Value          Relation
	ArtifactSHA256 string
	Ordinal        int
	SourceInHit    bool
	TargetInHit    bool
}

// ArmGroup holds candidates for one document eligibility group.
type ArmGroup struct {
	Dense    []Candidate
	Lexical  []Candidate
	RawANN   []Candidate
	Fallback bool
	Members  []Candidate
}

// Arms is the reusable candidate cache for a prepared query.
type Arms struct {
	Release           string
	RecipeHash        string
	QueryRecipeHash   string
	Validation        Validation
	Depths            Depths
	Date              Date
	AsOf              *Date
	IncludeNonCurrent bool
	Primary           ArmGroup
	NonCurrent        ArmGroup
}
