package textparse

// Scope describes a run of paragraphs subject to one article sequence check.
type Scope struct {
	Kind        string
	Accepted    bool
	FailedCheck string
}

// Provision is a citable span of source paragraphs.
type Provision struct {
	Kind, Number, Label, Heading, Anchor string
	ScopeIndex                           int
	Text                                 string
	StartOrdinal, EndOrdinal             int
	StartLine, EndLine                   int `json:",omitempty"`
	NodeID                               string
	ParentAnchor                         string `json:",omitempty"`
	Ordinal                              int    `json:",omitempty"`
}

// Counts summarizes input coverage and accepted child structure.
type Counts struct {
	Paragraphs, Characters, ProvisionCharacters, TableCharacters, Images int
	Structure                                                            *StructureCounts `json:",omitempty"`
}

// StructureCounts records accepted child spans and conservative rejections.
type StructureCounts struct {
	ArticlesSplit, Clauses, Points int
	Reasons                        map[string]int
}

// Result contains the provisions, scope checks, counts and quality gaps.
type Result struct {
	Provisions []Provision
	Scopes     []Scope
	Counts     Counts
	Gaps       []string
}
