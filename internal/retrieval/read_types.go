package retrieval

import (
	"encoding/json"
	"errors"
	"unicode/utf8"
)

// ErrTooManyItems rejects calls above the batch item budget.
var ErrTooManyItems = errors.New("too many read items")

// ErrInvalidReadLimits rejects inconsistent internal paging options.
var ErrInvalidReadLimits = errors.New("invalid read limits")

// ReadRequest selects scopes from one pinned release.
type ReadRequest struct {
	Release string     `json:"release"`
	Items   []ReadItem `json:"items,omitempty"`
	// PageCap and PartSize override MCP defaults. WholeDocument measures bytes;
	// other modes measure Unicode code points of serialized units.
	PageCap  int `json:"-"`
	PartSize int `json:"-"`
	// WholeDocument adds the outline and all text after the document entry.
	WholeDocument bool `json:"-"`
	// DocumentKey binds whole-document continuations to the reader route's key.
	DocumentKey string `json:"-"`
}

// Unit is one lead, provision, outline entry or relation, or a fixed part of it.
type Unit struct {
	Kind     string          `json:"kind"`
	Locator  *string         `json:"locator,omitempty"`
	Anchor   string          `json:"anchor,omitempty"`
	Citation string          `json:"citation,omitempty"`
	Label    string          `json:"label,omitempty"`
	Heading  string          `json:"heading,omitempty"`
	Text     string          `json:"text,omitempty"`
	Size     int             `json:"size,omitempty"`
	Relation *ListedRelation `json:"relation,omitempty"`
	Part     int             `json:"part,omitempty"`
	Parts    int             `json:"parts,omitempty"`
}

// ListedRelation adds source evidence to a stored relation pointer.
type ListedRelation struct {
	Pointer
	RawLabel         string `json:"raw_label,omitempty"`
	Evidence         string `json:"evidence,omitempty"`
	EvidenceComplete bool   `json:"evidence_complete"`
}

// ItemResult is one page of a read item's chain.
type ItemResult struct {
	// FetchSHA256 is the source document hash, excluded from read output.
	FetchSHA256 string         `json:"-"`
	Doc         string         `json:"doc,omitempty"`
	Document    *DocumentEntry `json:"document,omitempty"`
	Units       []Unit         `json:"units,omitempty"`
	Complete    bool           `json:"complete"`
	Next        string         `json:"next,omitempty"`
	NotRead     string         `json:"not_read,omitempty"`
	Error       string         `json:"error,omitempty"`
	Gaps        []Gap          `json:"gaps,omitempty"`
}

// ReadResult preserves request item order and identifies the pinned release.
type ReadResult struct {
	Release string             `json:"release"`
	Items   []ItemResult       `json:"items,omitempty"`
	Nodes   map[string]NodeRef `json:"nodes"`
}

type readSequence struct {
	source   Document
	doc      string
	document *DocumentEntry
	units    []Unit
	parts    map[int][]Unit
	gaps     []Gap
	offset   int
	more     bool
	complete bool
}

func readSize(v any) int {
	// Read values contain only JSON-compatible scalar fields and slices.
	b, _ := json.Marshal(v)
	return utf8.RuneCount(b)
}
