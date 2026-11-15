package retrieval

import (
	"encoding/json"
	"errors"
	"unicode/utf8"
)

var ErrTooManyItems = errors.New("too many read items")

var ErrInvalidReadLimits = errors.New("invalid read limits")

type ReadRequest struct {
	Release       string     `json:"release"`
	Items         []ReadItem `json:"items,omitempty"`
	PageCap       int        `json:"-"`
	PartSize      int        `json:"-"`
	WholeDocument bool       `json:"-"`
	DocumentKey   string     `json:"-"`
}

type Unit struct {
	Kind            string          `json:"kind"`
	Warnings        []Warning       `json:"warnings,omitempty"`
	WarningsOmitted int             `json:"warnings_omitted,omitempty"`
	Locator         *string         `json:"locator,omitempty"`
	Anchor          string          `json:"anchor,omitempty"`
	Citation        string          `json:"citation,omitempty"`
	Label           string          `json:"label,omitempty"`
	Heading         string          `json:"heading,omitempty"`
	Text            string          `json:"text,omitempty"`
	Size            int             `json:"size,omitempty"`
	Relation        *ListedRelation `json:"relation,omitempty"`
	Part            int             `json:"part,omitempty"`
	Parts           int             `json:"parts,omitempty"`
}

type ListedRelation struct {
	Pointer
	RawLabel         string `json:"raw_label,omitempty"`
	Evidence         string `json:"evidence,omitempty"`
	EvidenceComplete bool   `json:"evidence_complete"`
}

type ItemResult struct {
	Files       []SourceFile   `json:"-"`
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

type ReadResult struct {
	Release string             `json:"release"`
	Items   []ItemResult       `json:"items,omitempty"`
	Nodes   map[string]NodeRef `json:"nodes"`
}

type readSequence struct {
	files    []SourceFile
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
	b, _ := json.Marshal(v)
	return utf8.RuneCount(b)
}
