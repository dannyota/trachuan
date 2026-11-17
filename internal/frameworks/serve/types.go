package serve

import (
	"danny.vn/trachuan/internal/retrieval"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Caller struct {
	Readable          func(code, access string) bool
	AccessUnavailable bool
}

type Options struct{ CursorKey []byte }

type Request struct {
	Query   string
	Release string
	Limit   int
	Mode    string
	Filters map[string][]string
	Cursor  string
}

type Lineage struct {
	Supersedes string `json:"supersedes,omitempty"`
	Amends     string `json:"amends,omitempty"`
}

type Framework struct {
	Code      string  `json:"code"`
	Version   string  `json:"version"`
	Name      string  `json:"name"`
	Publisher string  `json:"publisher"`
	Access    string  `json:"access"`
	Readable  bool    `json:"readable"`
	Lineage   Lineage `json:"lineage,omitzero"`
}

type Hit struct {
	Handle        string   `json:"handle"`
	Citation      string   `json:"citation"`
	Kind          string   `json:"kind"`
	Title         string   `json:"title,omitempty"`
	Status        string   `json:"status,omitempty"`
	Path          []string `json:"path,omitempty"`
	Score         *float64 `json:"score,omitempty"`
	Lookup        *bool    `json:"lookup,omitempty"`
	Mappings      int      `json:"mappings"`
	TextWithheld  bool     `json:"text_withheld"`
	TitleOriginal string   `json:"title_original,omitempty"`
	Snippet       string   `json:"snippet,omitempty"`
}

type Gap struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
	Subject string `json:"subject,omitempty"`
	Detail  string `json:"detail"`
}

type Response struct {
	Release           string               `json:"release"`
	Abstain           bool                 `json:"abstain"`
	Reason            string               `json:"reason,omitempty"`
	Message           string               `json:"message,omitempty"`
	Frameworks        map[string]Framework `json:"frameworks,omitempty"`
	Hits              []Hit                `json:"hits,omitempty"`
	Gaps              []Gap                `json:"gaps,omitempty"`
	NextCursor        string               `json:"next_cursor,omitempty"`
	AccessSetVersion  string               `json:"access_set_version"`
	AccessUnavailable bool                 `json:"access_unavailable,omitempty"`
}

type Engine struct {
	db             *pgxpool.Pool
	embedder       retrieval.Embedder
	key            []byte
	release        string
	dense          retrieval.DenseRecipe
	lexical        lexical.Recipe
	recipe         recipe
	versions       []version
	partitions     map[int64]*partition
	partitionBytes int64
}

type version struct {
	id      int64
	handle  string
	aliases []string
	info    Framework
}

type failure string

func (f failure) Error() string { return string(f) }

const (
	ErrInvalidRequest failure = "invalid_request"
	ErrReleaseExpired failure = "release_expired"
	ErrCursorExpired  failure = "cursor_expired"
	ErrUnavailable    failure = "unavailable"
	ErrRateLimited    failure = "rate_limited"
)
