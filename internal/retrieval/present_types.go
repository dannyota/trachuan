package retrieval

// Detail selects the amount of evidence returned with each search hit.
type Detail string

const (
	// DetailCompact returns snippets for scanning.
	DetailCompact Detail = "compact"
	// DetailFull returns the recipe's full evidence.
	DetailFull Detail = "full"
)

// PresentedPathNode identifies a provision in a presented hit's ancestry.
type PresentedPathNode struct {
	Label   string `json:"label,omitempty"`
	Heading string `json:"heading,omitempty"`
	Anchor  string `json:"anchor,omitempty"`
}

// PresentedHit is the agent-facing presentation of a search hit.
type PresentedHit struct {
	Handle          string              `json:"handle,omitempty"`
	Doc             string              `json:"doc,omitempty"`
	Citation        string              `json:"citation,omitempty"`
	Path            []PresentedPathNode `json:"path,omitempty"`
	Snippet         string              `json:"snippet,omitempty"`
	Complete        bool                `json:"complete"`
	InFilter        *bool               `json:"in_filter,omitempty"`
	Nodes           []string            `json:"nodes"`
	Warnings        []Warning           `json:"warnings,omitempty"`
	WarningsOmitted int                 `json:"warnings_omitted,omitempty"`
	Gaps            []Gap               `json:"gaps,omitempty"`
	Locator         *string             `json:"locator,omitempty"`
	Text            string              `json:"text,omitempty"`
	Fragments       []Fragment          `json:"fragments,omitempty"`
	Score           *float64            `json:"score,omitempty"`
	Relations       []Relation          `json:"relations,omitempty"`
	Next            string              `json:"next,omitempty"`
}

// SearchResult contains presented hits and their shared document metadata.
type SearchResult struct {
	Release            string                   `json:"release,omitempty"`
	Abstain            bool                     `json:"abstain"`
	Reason             string                   `json:"reason,omitempty"`
	Message            string                   `json:"message,omitempty"`
	Documents          map[string]DocumentEntry `json:"documents,omitempty"`
	Hits               []PresentedHit           `json:"hits,omitempty"`
	NonCurrent         []PresentedHit           `json:"non_current,omitempty"`
	Gaps               []Gap                    `json:"gaps,omitempty"`
	OutsideFilterCount *int                     `json:"outside_filter_count,omitempty"`
	NextCursor         string                   `json:"next_cursor,omitempty"`
	Nodes              map[string]NodeRef       `json:"nodes"`
}
