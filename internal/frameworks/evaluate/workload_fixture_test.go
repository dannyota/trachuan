package evaluate

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/build"
	"danny.vn/trachuan/internal/frameworks/serve"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/jackc/pgx/v5"
)

type workloadCounts struct {
	embeds, opens, reads, mappings, searches int
	reducedSearches, calibrations            int
	embeddingTime, openTime                  time.Duration
}
type workloadQuery struct{ counts *workloadCounts }

func (q workloadQuery) Recipe(context.Context) (embedding.Recipe, error) {
	return embedding.QueryRecipe1(), nil
}
func (q workloadQuery) Embed(context.Context, string) (embedding.Vector, error) {
	start := time.Now()
	q.counts.embeds++
	v := make([]float32, embedding.QueryRecipe1().Dimensions)
	v[0] = 1
	q.counts.embeddingTime += time.Since(start)
	return embedding.Vector{Values: v, TokenCount: 4, RecipeHash: embedding.QueryRecipe1().Hash()}, nil
}

type workloadState map[string]map[string]bool

func (s workloadState) changed(code, class string) bool { return s[code][class] }

type workloadEngine struct {
	counts *workloadCounts
	state  workloadState
	query  retrieval.Embedder
	cases  map[string]GoldenCase
}

func (e workloadEngine) any(c serve.Caller, class string) bool {
	for _, code := range []string{"fw-alpha", "fw-gamma"} {
		if c.Readable(code, "licensed") && e.state.changed(code, class) {
			return true
		}
	}
	return false
}
func (e workloadEngine) Search(ctx context.Context, r serve.Request, c serve.Caller) (serve.Response, error) {
	e.counts.searches++
	if !c.Readable("fw-alpha", "licensed") || !c.Readable("fw-gamma", "licensed") {
		e.counts.reducedSearches++
	}
	if _, err := e.query.Embed(ctx, r.Query); err != nil {
		return serve.Response{}, err
	}
	score := 1.0
	if e.any(c, "body_index") {
		score = 2
	}
	response := serve.Response{Abstain: strings.HasPrefix(r.Query, "fixture-negative"), Hits: []serve.Hit{{Handle: e.cases[r.Query].Target, Score: &score}}}
	page := 0
	if r.Cursor != "" {
		page, _ = strconv.Atoi(r.Cursor)
	}
	if page < 2 {
		response.NextCursor = strconv.Itoa(page + 1)
	}
	return response, nil
}
func (e workloadEngine) Read(_ context.Context, r serve.ReadRequest, c serve.Caller) (serve.ReadResponse, error) {
	e.counts.reads++
	h := r.Items[0].Handle
	next := r.Items[0].Next
	if next != "" {
		h = strings.TrimSuffix(next, "|next")
	}
	code := versionCode(strings.SplitN(h, "#", 2)[0])
	text := "original"
	access := "licensed"
	if code == "fw-beta" {
		access = "free"
	}
	units := []serve.ReadUnit{}
	if c.Readable(code, access) {
		if e.state.changed(code, "control_text") {
			text = "changed"
		}
		units = append(units, serve.ReadUnit{Kind: "body", Text: text})
	}
	item := serve.ReadItemResult{Handle: h, Units: units, Complete: next != ""}
	if !item.Complete {
		item.Next = h + "|next"
	}
	return serve.ReadResponse{Items: []serve.ReadItemResult{item}}, nil
}
func (e workloadEngine) Mappings(_ context.Context, _ serve.MappingsRequest, c serve.Caller) (serve.MappingsResponse, error) {
	e.counts.mappings++
	evidence := ""
	if e.any(c, "mapping_evidence") {
		evidence = "changed"
	}
	return serve.MappingsResponse{Edges: []serve.MappingEdge{{Evidence: evidence}}}, nil
}
func (e workloadEngine) CorpusStatus(_ context.Context, _ serve.StatusRequest, c serve.Caller) (serve.StatusResponse, error) {
	r := serve.StatusResponse{RetrievalRecipeStatus: "measured", Versions: map[string]serve.VersionStatus{}}
	for _, code := range []string{"fw-alpha", "fw-beta", "fw-gamma"} {
		access := "licensed"
		if code == "fw-beta" {
			access = "free"
		}
		v := serve.VersionStatus{Access: access, Readable: c.Readable(code, access)}
		if v.Readable {
			source := "structured"
			if e.state.changed(code, "source_files") {
				source = "ocr"
			}
			v.Sources = []serve.SourceStatus{{Name: "invented", TextSource: source, TextComplete: true}}
		}
		r.Versions["fw:"+code+":1"] = v
	}
	return r, nil
}
func (e workloadEngine) QualityGaps(_ context.Context, _ serve.GapsRequest, c serve.Caller) (serve.GapsResponse, error) {
	r := serve.GapsResponse{}
	if e.any(c, "text_gaps") {
		r.Gaps = []serve.Gap{{Kind: "text_incomplete", Detail: "1"}}
	}
	return r, nil
}
func (e workloadEngine) CalibrationSearch(ctx context.Context, q string) (*float64, bool, error) {
	e.counts.calibrations++
	if _, err := e.query.Embed(ctx, q); err != nil {
		return nil, false, err
	}
	value := .8
	if strings.HasPrefix(q, "fixture-negative") {
		value = .2
	}
	return &value, false, nil
}
func (workloadEngine) CalibrationRecipeHash() string { return "fixture" }
func workloadRequest() EvaluationRequest {
	req := EvaluationRequest{DatabaseRequest: build.DatabaseRequest{Rel: "20990101"}, CaseFileSHA256: strings.Repeat("a", 64), Calibration: &build.CalibrationRecord{Threshold: .5}}
	for i := range 60 {
		code, access := "fw-beta", "free"
		if i >= 33 && i < 39 {
			code, access = "fw-alpha", "licensed"
		}
		if i >= 39 && i < 45 {
			code, access = "fw-gamma", "licensed"
		}
		c := GoldenCase{ID: fmt.Sprintf("case-%02d", i), Question: fmt.Sprintf("fixture-positive-%02d", i), Code: code, Version: "1", Access: access, Adjudication: "accepted"}
		if i >= 45 {
			c.Negative = true
			c.Question = fmt.Sprintf("fixture-negative-%02d", i)
		} else {
			c.Target = fmt.Sprintf("fw:%s:1#A-%04d", code, i)
			req.Controls = append(req.Controls, Control{Code: code, Version: "1", Citation: fmt.Sprintf("A-%04d", i)})
		}
		req.Cases = append(req.Cases, c)
	}
	for i := 45; i < 3943; i++ {
		req.Controls = append(req.Controls, Control{Code: "fw-alpha", Version: "1", Citation: fmt.Sprintf("A-%04d", i)})
	}
	return req
}
func workloadCopies(counts *workloadCounts, query retrieval.Embedder, cases []GoldenCase) invariantCopies {
	state := workloadState{}
	byQuestion := map[string]GoldenCase{}
	for _, c := range cases {
		byQuestion[c.Question] = c
	}
	return invariantCopies{
		restore: func(ctx context.Context, _ build.DatabaseRequest, _ string, _ bool, use func(context.Context, *pgx.Conn) error) error {
			state = workloadState{}
			return use(ctx, nil)
		},
		open: func(context.Context, *pgx.Conn, []byte) (Engine, func(), error) {
			start := time.Now()
			counts.opens++
			e := workloadEngine{counts: counts, state: state, query: query, cases: byQuestion}
			counts.openTime += time.Since(start)
			return e, func() {}, nil
		},
		mutate: func(_ context.Context, _ *pgx.Conn, caller serve.Caller, class string) error {
			for _, code := range []string{"fw-alpha", "fw-gamma"} {
				if !caller.Readable(code, "licensed") {
					if state[code] == nil {
						state[code] = map[string]bool{}
					}
					state[code][class] = true
				}
			}
			return nil
		},
	}
}
