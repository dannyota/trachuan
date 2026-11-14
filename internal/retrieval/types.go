package retrieval

import (
	"context"
	"time"

	"danny.vn/trachuan/internal/embedding"
)

type Embedder interface {
	Recipe(context.Context) (embedding.Recipe, error)
	Embed(context.Context, string) (embedding.Vector, error)
}

type Profile string

type Mode string

type ArmMode string

type Date struct {
	Year  int
	Month time.Month
	Day   int
}

type Request struct {
	Instant     time.Time `json:"-"`
	Query       string
	Limit       int
	InForceOnly *bool
	AsOf        *Date
	Mode        Mode
	Arms        ArmMode
	Nodes       []string
	DocTypes    []string
	Issuers     []string
	Documents   []string
	Cursor      string
}

type Prepared struct {
	Request    Request
	Validation Validation
	Vector     *embedding.Vector
}

type Options struct {
	Now        func() time.Time
	Profile    Profile
	Documents  DocumentCache
	Sequences  SequenceCache
	Candidates CandidateCache
}

type Depths struct {
	Dense   int
	Lexical int
}

type Identity struct {
	Country     string `json:"country"`
	DocumentKey string `json:"document_key"`
	Anchor      string `json:"anchor"`
}

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
	Locator  *string
}

type Document struct {
	FetchSHA256      string
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

type Candidate struct {
	ChunkID         int64
	Ordinal         int
	Identity        Identity
	Group           Identity
	Rank            int
	Cosine          float64
	BM25            float64
	ProvisionID     int64
	GroupID         int64
	Content         string
	ContextPrefix   string
	Spans           []byte
	Provision       Provision
	Document        Document
	Ancestors       []Provision
	Descendants     []Provision
	Relations       []SourceRelation
	Warnings        []Warning
	WarningsOmitted int
}

type SourceRelation struct {
	Value          Relation
	ArtifactSHA256 string
	Ordinal        int
	SourceInHit    bool
	TargetInHit    bool
}

type ArmGroup struct {
	Dense    []Candidate
	Lexical  []Candidate
	RawANN   []Candidate
	Fallback bool
	Members  []Candidate
}

type Arms struct {
	Release           string
	RecipeHash        string
	QueryRecipeHash   string
	Validation        Validation
	Depths            Depths
	Date              Date
	AsOf              *Date
	IncludeNonCurrent bool
	EffectiveNodes    map[int64][]string
	Nodes             map[string]NodeRef
	Articles          map[int64]Candidate `json:",omitempty"`
	Primary           ArmGroup
	NonCurrent        ArmGroup
}
