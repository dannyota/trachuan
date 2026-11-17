package classify

type Node struct {
	Code   string `json:"code"`
	Name   string `json:"name"`
	Parent string `json:"parent_code"`
	Level  int    `json:"level"`
	Active bool   `json:"active"`
	Origin string `json:"origin"`
}

type Article struct {
	DocumentKey    string `json:"-"`
	Anchor         string `json:"anchor"`
	Heading        string `json:"heading"`
	Text           string `json:"text"`
	Chapter        string `json:"-"`
	ChapterHeading string `json:"chapter_heading,omitempty"`
	OwnText        string `json:"-"`
}

type ArticleRequest struct {
	DocumentKey string    `json:"document_key"`
	Articles    []Article `json:"articles"`
	Nodes       []Node    `json:"allowed_subtrees"`
	InputTokens int64     `json:"counted_input_tokens"`
	Step        int       `json:"step,omitempty"`
	Family      string    `json:"family,omitempty"`
}

type CodeConfidence struct {
	Code       string  `json:"code"`
	Confidence float64 `json:"confidence"`
}

type ArticleOutcome struct {
	Outcome        string           `json:"outcome"`
	Codes          []CodeConfidence `json:"codes"`
	Dropped        []string         `json:"dropped_codes"`
	FailedFamilies *int             `json:"failed_families,omitempty"`
}

type ArticleRecord struct {
	DocumentKey string         `json:"document_key"`
	Anchor      string         `json:"anchor"`
	Outcome     ArticleOutcome `json:"result"`
}

type ArticlePlan struct {
	Requests []ArticleRequest `json:"requests"`
	Skipped  []ArticleRecord  `json:"skipped"`
}
