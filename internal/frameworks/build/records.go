package build

type Control struct {
	Code          string   `json:"code"`
	Version       string   `json:"version"`
	Citation      string   `json:"citation"`
	CitationNorm  string   `json:"citation_norm"`
	Kind          string   `json:"kind"`
	Parent        *string  `json:"parent"`
	Ordinal       int      `json:"ordinal"`
	Status        string   `json:"status"`
	Title         *string  `json:"title"`
	TitleOriginal string   `json:"title_original"`
	Body          string   `json:"body"`
	Notes         []string `json:"notes"`
	Tables        []string `json:"tables"`
	Supersedes    *string  `json:"supersedes"`
	Amends        *string  `json:"amends"`
}

type Gap struct {
	Kind     string  `json:"kind"`
	Code     string  `json:"code"`
	Version  string  `json:"version"`
	Citation *string `json:"citation"`
	Count    int     `json:"count"`
}

type Chunk struct {
	Index         string `json:"index"`
	DocumentKey   string `json:"document_key"`
	Ordinal       int    `json:"ordinal"`
	Citation      string `json:"citation"`
	ChunkIndex    int    `json:"chunk_index"`
	ContextPrefix string `json:"context_prefix"`
	Content       string `json:"content"`
}
