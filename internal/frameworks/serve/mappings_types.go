package serve

type MappingEnd struct {
	Handle   string `json:"handle,omitempty"`
	Code     string `json:"code,omitempty"`
	Citation string `json:"citation"`
	Version  string `json:"version"`
	Title    string `json:"title,omitempty"`
}

type MappingEdge struct {
	From              MappingEnd `json:"from"`
	To                MappingEnd `json:"to"`
	Relation          string     `json:"relation"`
	Source            string     `json:"source"`
	Resolved          bool       `json:"resolved"`
	Evidence          string     `json:"evidence,omitempty"`
	EvidenceTruncated *bool      `json:"evidence_truncated,omitempty"`
}

type MappingsRequest struct {
	Release, Handle, From, To string
	Relations                 []string
	Limit                     int
	Cursor                    string
}

type MappingsResponse struct {
	Metadata
	Edges      []MappingEdge `json:"edges,omitempty"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

type mappingRow struct {
	id    int64
	key   []string
	value MappingEdge
}
