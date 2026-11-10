package embed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/internal/embedding"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type fake struct {
	calls        []string
	bodies       [][]string
	counts       map[string]int
	status       int
	embedStatus  int
	badHash      bool
	badHashEmbed bool
	transport    bool
}

func (f *fake) client(r embedding.Recipe) *embedding.Client {
	return embedding.NewClient("http://embedder", &http.Client{Transport: roundTrip(func(req *http.Request) (*http.Response, error) {
		if f.transport {
			return nil, errors.New("transport")
		}
		f.calls = append(f.calls, req.URL.Path)
		if f.embedStatus != 0 && req.URL.Path == "/v1/embed" {
			return &http.Response{StatusCode: f.embedStatus, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"failure"}}`)), Header: make(http.Header)}, nil
		}
		if f.status != 0 {
			return &http.Response{StatusCode: f.status, Body: io.NopCloser(strings.NewReader(`{"error":{"code":"failure"}}`)), Header: make(http.Header)}, nil
		}
		var out any
		hash := r.Hash()
		if f.badHash || (f.badHashEmbed && req.URL.Path == "/v1/embed") {
			hash = strings.Repeat("a", 64)
		}
		if req.URL.Path == "/v1/recipe" {
			out = embedding.Report{ProtocolVersion: 1, RecipeHash: hash, Recipe: r}
		} else {
			var body struct {
				Texts []string `json:"texts"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
				return nil, err
			}
			f.bodies = append(f.bodies, body.Texts)
			counts := make([]int, len(body.Texts))
			data := make([]any, len(body.Texts))
			for i, s := range body.Texts {
				n := f.counts[s]
				if n == 0 {
					n = 2
				}
				counts[i] = n
				data[i] = map[string]any{"index": i, "token_count": n, "embedding": []float32{1, 0}}
			}
			if req.URL.Path == "/v1/tokenize" {
				out = map[string]any{"recipe_hash": hash, "counts": counts}
			} else {
				out = map[string]any{"recipe_hash": hash, "dimensions": 2, "data": data}
			}
		}
		b, _ := json.Marshal(out)
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(b))), Header: make(http.Header)}, nil
	})})
}
func recipe() embedding.Recipe {
	r := embedding.DocumentRecipe1()
	r.Dimensions = 2
	r.TokenCap = 4
	return r
}
func run(t *testing.T, f *fake, chunks ...chunk.Chunk) (Index, []byte, error) {
	t.Helper()
	r := recipe()
	s := New(f.client(r), r)
	return s.build(context.Background(), chunk.File{Version: 1, DocumentKey: "doc", Chunks: chunks}, chunk.Coverage{}, strings.Repeat("b", 64))
}
func TestEmbed_InputIsStrippedPrefixNewlineContent(t *testing.T) {
	f := &fake{}
	_, _, err := run(t, f, chunk.Chunk{Ordinal: 1, ContextPrefix: "  prefix ", Content: " text  "})
	if err != nil || len(f.bodies) != 2 || f.bodies[0][0] != "prefix\ntext" {
		t.Fatalf("bodies=%v err=%v", f.bodies, err)
	}
}
func TestEmbed_TokenizeThenOmitOverCap(t *testing.T) {
	f := &fake{counts: map[string]int{"\nlong": 5}}
	idx, _, err := run(t, f, chunk.Chunk{Ordinal: 0, Content: "long"})
	if err != nil || len(idx.Rejected) != 1 || idx.Rejected[0].Kind != "embedding_input_too_long" || *idx.Rejected[0].TokenCount != 5 || len(f.calls) != 1 {
		t.Fatalf("index=%+v calls=%v err=%v", idx, f.calls, err)
	}
}
func TestEmbed_PacksBatchesUnderByteCap(t *testing.T) {
	f := &fake{}
	chunks := make([]chunk.Chunk, 18)
	for i := range chunks {
		chunks[i] = chunk.Chunk{Ordinal: i, Content: strings.Repeat("x", 70000)}
	}
	_, _, err := run(t, f, chunks...)
	if err != nil {
		t.Fatal(err)
	}
	for _, texts := range f.bodies {
		b, _ := json.Marshal(struct {
			RecipeHash string   `json:"recipe_hash"`
			Texts      []string `json:"texts"`
		}{recipe().Hash(), texts})
		if len(texts) > 16 || len(b) > 1048576 {
			t.Fatal("batch too large")
		}
	}
}
func TestEmbed_SingleInputOverByteCapIsGap(t *testing.T) {
	f := &fake{}
	idx, _, err := run(t, f, chunk.Chunk{Content: strings.Repeat("x", 1048576)})
	if err != nil || len(idx.Rejected) != 1 || idx.Rejected[0].Kind != "embedding_input_bytes_exceeded" || idx.Rejected[0].TokenCount != nil || len(f.calls) != 0 {
		t.Fatalf("index=%+v err=%v", idx, err)
	}
}
func TestEmbed_RejectedMiddleOrdinalKeepsAlignment(t *testing.T) {
	f := &fake{counts: map[string]int{"\nb": 5}}
	idx, _, err := run(t, f, chunk.Chunk{Ordinal: 0, Content: "a"}, chunk.Chunk{Ordinal: 1, Content: "b"}, chunk.Chunk{Ordinal: 2, Content: "c"})
	if err != nil || len(idx.Rows) != 2 || idx.Rows[1].ChunkOrdinal != 2 || idx.Rows[1].InputSHA256 != hashText("\nc") {
		t.Fatalf("index=%+v err=%v", idx, err)
	}
}
func TestEmbed_PlanRefusesRecipeMismatch(t *testing.T) {
	r := recipe()
	other := r
	other.Model = "wrong"
	s := New((&fake{}).client(other), r)
	_, err := s.Plan(context.Background(), stage.Env{}, stage.Run{})
	if err == nil {
		t.Fatal("expected mismatch")
	}
}
func TestEmbed_MidRunRecipeChangeFailsItem(t *testing.T) {
	f := &fake{badHashEmbed: true}
	_, _, err := run(t, f, chunk.Chunk{Content: "a"})
	var retry *stage.RetryableError
	if err == nil || errors.As(err, &retry) || len(f.calls) != 2 {
		t.Fatalf("calls=%v error=%v", f.calls, err)
	}
}
func TestEmbed_RetryableStatuses(t *testing.T) {
	for _, status := range []int{429, 503, 504, 400, 409, 415, 422} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			f := &fake{status: status}
			_, _, err := run(t, f, chunk.Chunk{Content: "a"})
			var retry *stage.RetryableError
			if err == nil || errors.As(err, &retry) != (status == 429 || status == 503 || status == 504) {
				t.Fatalf("error=%v", err)
			}
		})
	}
	f := &fake{transport: true}
	_, _, err := run(t, f, chunk.Chunk{Content: "a"})
	var retry *stage.RetryableError
	if !errors.As(err, &retry) {
		t.Fatalf("error=%v", err)
	}
}
func TestEmbed_EmptyChunkArtifactNoCalls(t *testing.T) {
	f := &fake{}
	idx, b, err := run(t, f)
	if err != nil || len(idx.Rows) != 0 || len(b) != 0 || len(f.calls) != 0 {
		t.Fatalf("index=%+v calls=%v err=%v", idx, f.calls, err)
	}
}
func TestArtifact_SizeAndNormChecked(t *testing.T) {
	if err := validateVectors([]byte{0}, 1, 2); err == nil {
		t.Fatal("accepted wrong size")
	}
	if err := validateVectors(make([]byte, 8), 1, 2); err == nil {
		t.Fatal("accepted zero norm")
	}
}

func TestEmbed_MalformedResponseIsNotSourceData(t *testing.T) {
	r := recipe()
	client := embedding.NewClient("http://embedder", &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader("!")), Header: make(http.Header)}, nil
	})})
	_, _, err := New(client, r).build(context.Background(), chunk.File{Chunks: []chunk.Chunk{{Content: "a"}}}, chunk.Coverage{}, strings.Repeat("b", 64))
	var syntax *json.SyntaxError
	if err == nil || errors.As(err, &syntax) || errors.Is(err, stage.ErrSourceData) {
		t.Fatalf("malformed output classified as source data: %v", err)
	}
}

func TestArtifact_PublishedVectors(t *testing.T) {
	f := &fake{}
	idx, vectors, err := run(t, f, chunk.Chunk{Ordinal: 0, Content: "a"}, chunk.Chunk{Ordinal: 1, Content: "b"})
	if err != nil {
		t.Fatal(err)
	}
	store := artifact.Store{Root: t.TempDir()}
	published, err := store.Publish(context.Background(), "embed", strings.Repeat("b", 64), strings.Repeat("c", 64), func(dir string) error {
		for _, entry := range []struct {
			name string
			data any
		}{{"index.json", idx}, {"report.json", makeReport(idx, chunk.Coverage{})}} {
			b, e := json.Marshal(entry.data)
			if e != nil {
				return e
			}
			if e = os.WriteFile(filepath.Join(dir, entry.name), b, 0600); e != nil {
				return e
			}
		}
		return os.WriteFile(filepath.Join(dir, "vectors.f32"), vectors, 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	dir := store.Abs(published.Path)
	got, err := ReadArtifact(dir)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := OpenVectors(dir, got)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reader.Close() }()
	row, err := reader.Row(1)
	if err != nil || len(row) != 2 || row[0] != 1 {
		t.Fatalf("row=%v err=%v", row, err)
	}
	if err = os.WriteFile(filepath.Join(dir, "vectors.f32"), []byte{0}, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = ReadArtifact(dir); err == nil {
		t.Fatal("accepted tampered vectors")
	}
}

func TestEmbed_Rejects422AfterTokenize(t *testing.T) {
	f := &fake{embedStatus: 422}
	_, _, err := run(t, f, chunk.Chunk{Content: "a"})
	var retry *stage.RetryableError
	if err == nil || errors.As(err, &retry) || len(f.calls) != 2 || f.calls[1] != "/v1/embed" {
		t.Fatalf("calls=%v error=%v", f.calls, err)
	}
}
