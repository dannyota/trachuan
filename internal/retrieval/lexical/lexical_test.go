package lexical

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"testing"
)

type textIterator struct {
	texts []string
	pos   int
}

func (i *textIterator) Next(_ context.Context) (string, bool, error) {
	if i.pos == len(i.texts) {
		return "", false, nil
	}
	s := i.texts[i.pos]
	i.pos++
	return s, true, nil
}
func train(t *testing.T, texts ...string) Stats {
	t.Helper()
	s, err := Train(context.Background(), &textIterator{texts: texts}, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func TestTokenize_FoldsVietnamese(t *testing.T) {
	got, err := Tokenize("Điều 3. Ngân hàng", Recipe1())
	if err != nil || !reflect.DeepEqual(got, []string{"dieu", "3", "ngan", "hang"}) {
		t.Fatalf("got %v, %v", got, err)
	}
}
func TestTokenize_SplitsPunctuationAndKeepsDigits(t *testing.T) {
	got, err := Tokenize("50/2024/TT-NHNN", Recipe1())
	if err != nil || !reflect.DeepEqual(got, []string{"50", "2024", "tt", "nhnn"}) {
		t.Fatalf("got %v, %v", got, err)
	}
}
func TestDimension_FNV1aModuloPlusOne(t *testing.T) {
	for _, tc := range []struct {
		token string
		want  int32
	}{{"", 826822}, {"a", 796973}, {"hello", 994476}} {
		if got := Dimension(tc.token, Recipe1()); got != tc.want || got < 1 || got > 1048576 {
			t.Errorf("%q: %d, want %d", tc.token, got, tc.want)
		}
	}
}
func TestTrain_CountsDFOncePerChunk(t *testing.T) {
	s := train(t, "a a b", "b", "")
	if s.N != 3 || s.TotalTokens != 4 || s.AvgDL != 4.0/3 || !reflect.DeepEqual(s.DF, map[string]int64{"a": 1, "b": 2}) {
		t.Fatalf("%+v", s)
	}
}
func TestTrain_EmptyCorpusAvgDLOne(t *testing.T) {
	for _, texts := range [][]string{nil, {"", "..."}} {
		s := train(t, texts...)
		if s.AvgDL != 1 || s.TotalTokens != 0 {
			t.Fatalf("%+v", s)
		}
	}
}
func TestDocument_BM25Weight(t *testing.T) {
	s := train(t, "a a b", "b")
	v, err := Document("a a b", s, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	var got float32
	for _, e := range v {
		if e.Index == Dimension("a", Recipe1()) {
			got = e.Value
		}
	}
	// N=2, df(a)=1, tf=2, dl=3, avgdl=2: ln(2)*2*2.2/(2+1.2*(0.25+0.75*3/2)).
	want := math.Log(2) * 2 * 2.2 / (2 + 1.2*(0.25+0.75*3.0/2))
	if math.Abs(float64(got)-want) > 1e-6 {
		t.Fatalf("got %v want %v", got, want)
	}
}
func collision() (string, string) {
	seen := map[int32]string{}
	for i := 0; ; i++ {
		s := fmt.Sprintf("t%d", i)
		d := Dimension(s, Recipe1())
		if prev, ok := seen[d]; ok {
			return prev, s
		}
		seen[d] = s
	}
}
func TestDocument_SumsCollisionsInSortedTokenOrder(t *testing.T) {
	a, b := collision()
	s := train(t, a+" "+b, a)
	both, err := Document(b+" "+a, s, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	// Both terms have tf=1 and dl=2.
	denom := 1 + 1.2*(0.25+0.75*2/1.5)
	idf := func(n, df float64) float64 { return math.Log(1 + (n-df+0.5)/(df+0.5)) }
	// a appears in both chunks (df 2), b in one (df 1); N is 2.
	want := float32(idf(2, 2)*2.2/denom + idf(2, 1)*2.2/denom)
	if len(both) != 1 || both[0].Value != want {
		t.Fatalf("%v want %v", both, want)
	}
}
func TestQuery_HashedPresenceIgnoresFrequency(t *testing.T) {
	a, b := collision()
	v, err := Query(a+" "+a+" "+b, Recipe1())
	if err != nil || len(v) != 1 || v[0].Value != 1 {
		t.Fatalf("%v %v", v, err)
	}
}
func TestEncode_QueryDocumentParity(t *testing.T) {
	text := "Điều 50/2024/TT-NHNN điều"
	s := train(t, text)
	d, err := Document(text, s, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	q, err := Query(text, Recipe1())
	if err != nil {
		t.Fatal(err)
	}
	indices := map[int32]bool{}
	for _, e := range d {
		indices[e.Index] = true
	}
	for _, e := range q {
		if !indices[e.Index] {
			t.Fatalf("missing %d", e.Index)
		}
	}
}
func TestDocument_RejectsOverNonzeroLimit(t *testing.T) {
	dims := map[int32]bool{}
	var tokens []string
	for i := 0; len(tokens) < 16001; i++ {
		s := fmt.Sprintf("t%d", i)
		d := Dimension(s, Recipe1())
		if !dims[d] {
			dims[d] = true
			tokens = append(tokens, s)
		}
	}
	text := strings.Join(tokens, " ")
	s := train(t, text)
	if _, err := Document(text, s, Recipe1()); err == nil {
		t.Fatal("expected limit error")
	}
}
func TestRecipe_UnknownAlgorithmRejected(t *testing.T) {
	r := Recipe1()
	r.Algorithm = "unknown"
	if _, err := Tokenize("a", r); err == nil {
		t.Fatal("accepted unknown algorithm")
	}
	if _, err := Train(context.Background(), &textIterator{}, r); err == nil {
		t.Fatal("accepted unknown algorithm")
	}
	if _, err := Document("a", Stats{}, r); err == nil {
		t.Fatal("accepted unknown algorithm")
	}
	if _, err := Query("a", r); err == nil {
		t.Fatal("accepted unknown algorithm")
	}
}
func TestRecipe_JSONAndHash(t *testing.T) {
	r := Recipe1()
	b, err := r.JSON()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"version":1,"algorithm":"bm25-vn-fold/1","input":"content-space-prefix","normalization":"lower-nfd-mn-dd/1","tokenizer":"unicode-letter-digit/1","hash":"fnv1a32","dimensions":1048576,"k1":1.2,"b":0.75,"query":"hashed-presence","collision":"sum-document"}`
	if string(b) != want {
		t.Fatalf("%s", b)
	}
	sum := sha256.Sum256(b)
	if r.Hash() != hex.EncodeToString(sum[:]) {
		t.Fatal("wrong hash")
	}
}
func TestStats_Summary(t *testing.T) {
	a, b := collision()
	s := train(t, a+" "+b, a, "z")
	summary := s.Summary(Recipe1(), []int{1, 2, 3, 4})
	raw, err := json.Marshal(summary)
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version    int                         `json:"version"`
		N          int64                       `json:"N"`
		Total      int64                       `json:"total_tokens"`
		Vocabulary string                      `json:"vocabulary_sha256"`
		DF         string                      `json:"df_sha256"`
		Distinct   int                         `json:"distinct_terms"`
		Occupied   int                         `json:"occupied_dimensions"`
		Collision  int                         `json:"collision_terms"`
		NNZ        struct{ P50, P95, Max int } `json:"nnz"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	terms := []string{a, b, "z"}
	sort.Strings(terms)
	vocab := make([][2]any, 0, 3)
	df := make([][2]any, 0, 3)
	for _, term := range terms {
		vocab = append(vocab, [2]any{term, Dimension(term, Recipe1())})
		df = append(df, [2]any{term, s.DF[term]})
	}
	vb, _ := json.Marshal(vocab)
	db, _ := json.Marshal(df)
	vh := sha256.Sum256(vb)
	dh := sha256.Sum256(db)
	if got.Version != 1 || got.N != 3 || got.Total != 4 || got.Vocabulary != hex.EncodeToString(vh[:]) || got.DF != hex.EncodeToString(dh[:]) || got.Distinct != 3 || got.Occupied != 2 || got.Collision != 1 || got.NNZ.P50 != 2 || got.NNZ.P95 != 4 || got.NNZ.Max != 4 {
		t.Fatalf("%s", raw)
	}
}
