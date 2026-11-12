// Package textrule parses explicit provision targets in Vietnamese relation sentences.
package textrule

// Target records a requested provision path and the document identity stated in text.
type Target struct {
	Anchor          string
	Number          string
	IssuerCode      string
	StatedIssueDate *string
}

// Match records one fully parsed relation sentence.
type Match struct {
	Kind       string
	VerbPhrase string
	Sentence   string
	Targets    []Target
}

// Gap records why a verb-led sentence or the whole provision was rejected.
type Gap struct {
	Name   string
	Detail string
}
