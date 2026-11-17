package serve

type ReadItem struct {
	Handle string `json:"handle,omitempty"`
	Next   string `json:"next,omitempty"`
}

type ReadRequest struct {
	Release        string
	Items          []ReadItem
	ExpectedHandle string
}

type ReadHead struct {
	Handle       string   `json:"handle"`
	Citation     string   `json:"citation"`
	Kind         string   `json:"kind"`
	Path         []string `json:"path,omitempty"`
	Title        string   `json:"title,omitempty"`
	Status       string   `json:"status,omitempty"`
	Lineage      Lineage  `json:"lineage,omitzero"`
	Access       string   `json:"access"`
	TextWithheld bool     `json:"text_withheld"`
}

type Outline struct {
	Citation string `json:"citation"`
	Kind     string `json:"kind,omitempty"`
	Title    string `json:"title,omitempty"`
}

type ReadUnit struct {
	Kind    string       `json:"kind"`
	Head    *ReadHead    `json:"head,omitempty"`
	Version *Framework   `json:"version,omitempty"`
	Text    string       `json:"text,omitempty"`
	Edge    *MappingEdge `json:"edge,omitempty"`
	Outline *Outline     `json:"outline,omitempty"`
	Part    int          `json:"part,omitempty"`
	Parts   int          `json:"parts,omitempty"`
}

type ReadItemResult struct {
	Handle   string     `json:"handle,omitempty"`
	Units    []ReadUnit `json:"units,omitempty"`
	Complete bool       `json:"complete"`
	Next     string     `json:"next,omitempty"`
	NotRead  string     `json:"not_read,omitempty"`
	Error    string     `json:"error,omitempty"`
}

type ReadResponse struct {
	Metadata
	Items []ReadItemResult `json:"items,omitempty"`
}
