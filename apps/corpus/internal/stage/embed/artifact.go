package embed

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/internal/embedding"
)

// Index describes the vectors and rejected chunk ordinals.
type Index struct {
	Version     int              `json:"version"`
	DocumentKey string           `json:"document_key"`
	ChunkSHA256 string           `json:"chunk_sha256"`
	RecipeHash  string           `json:"recipe_hash"`
	Recipe      embedding.Recipe `json:"recipe"`
	Dimensions  int              `json:"dimensions"`
	Rows        []Row            `json:"rows"`
	Rejected    []Rejected       `json:"rejected"`
}

// Row maps a vector to its chunk and input.
type Row struct {
	Row          int    `json:"row"`
	ChunkOrdinal int    `json:"chunk_ordinal"`
	TokenCount   int    `json:"token_count"`
	InputSHA256  string `json:"input_sha256"`
}

// Rejected records an input omitted from the vectors.
type Rejected struct {
	ChunkOrdinal int    `json:"chunk_ordinal"`
	TokenCount   *int   `json:"token_count"`
	Kind         string `json:"kind"`
}
type reportGap struct {
	Kind   string `json:"kind"`
	Detail string `json:"detail"`
}
type report struct {
	Version  int         `json:"version"`
	Rows     int         `json:"rows"`
	Rejected int         `json:"rejected"`
	Gaps     []reportGap `json:"gaps"`
}

func makeReport(idx Index, coverage chunk.Coverage) report {
	r := report{Version: 1, Rows: len(idx.Rows), Rejected: len(idx.Rejected), Gaps: []reportGap{}}
	for _, g := range coverage.Gaps {
		r.Gaps = append(r.Gaps, reportGap{Kind: g.Kind, Detail: g.Detail})
	}
	for _, v := range idx.Rejected {
		detail := fmt.Sprintf("chunk ordinal %d", v.ChunkOrdinal)
		if v.TokenCount != nil {
			detail += fmt.Sprintf(", token count %d", *v.TokenCount)
		}
		r.Gaps = append(r.Gaps, reportGap{Kind: v.Kind, Detail: detail})
	}
	return r
}
func decodeFile(dir, name string, dst any) error {
	f, err := os.Open(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("open %s: %w", name, err)
	}
	defer func() { _ = f.Close() }()
	d := json.NewDecoder(f)
	d.DisallowUnknownFields()
	if err = d.Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	var extra any
	if err = d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("trailing JSON in %s", name)
	}
	return nil
}
func validHash(s string) bool {
	b, err := hex.DecodeString(s)
	return err == nil && len(b) == 32 && s == fmt.Sprintf("%x", b)
}

// ReadArtifact verifies and reads the embedding index.
func ReadArtifact(dir string) (Index, error) {
	var idx Index
	if _, err := artifact.Verify(dir); err != nil {
		return idx, fmt.Errorf("verify embed artifact: %w", err)
	}
	return readVerifiedIndex(dir)
}

func readVerifiedIndex(dir string) (Index, error) {
	var idx Index
	if err := decodeFile(dir, "index.json", &idx); err != nil {
		return idx, err
	}
	if idx.Version != 1 || idx.DocumentKey == "" || !validHash(idx.ChunkSHA256) || idx.RecipeHash != idx.Recipe.Hash() || idx.Dimensions <= 0 || idx.Dimensions != idx.Recipe.Dimensions || idx.Rows == nil || idx.Rejected == nil {
		return idx, fmt.Errorf("invalid embedding index")
	}
	seen := map[int]bool{}
	prev := -1
	for i, r := range idx.Rows {
		if r.Row != i || r.ChunkOrdinal <= prev || r.TokenCount <= 0 || r.TokenCount > idx.Recipe.TokenCap || !validHash(r.InputSHA256) {
			return idx, fmt.Errorf("invalid embedding row")
		}
		seen[r.ChunkOrdinal] = true
		prev = r.ChunkOrdinal
	}
	prev = -1
	for _, r := range idx.Rejected {
		if r.ChunkOrdinal <= prev || seen[r.ChunkOrdinal] || (r.Kind == "embedding_input_too_long" && (r.TokenCount == nil || *r.TokenCount <= idx.Recipe.TokenCap)) || (r.Kind == "embedding_input_bytes_exceeded" && r.TokenCount != nil) || (r.Kind != "embedding_input_too_long" && r.Kind != "embedding_input_bytes_exceeded") {
			return idx, fmt.Errorf("invalid rejected row")
		}
		seen[r.ChunkOrdinal] = true
		prev = r.ChunkOrdinal
	}
	return idx, nil
}
func readReport(dir string) (report, error) {
	var r report
	err := decodeFile(dir, "report.json", &r)
	if err != nil {
		return r, err
	}
	if r.Version != 1 || r.Rows < 0 || r.Rejected < 0 || r.Gaps == nil {
		return r, fmt.Errorf("invalid embedding report")
	}
	return r, nil
}
func validateVectors(data []byte, rows, dimensions int) error {
	if rows < 0 || dimensions <= 0 || int64(len(data)) != int64(rows)*int64(dimensions)*4 {
		return fmt.Errorf("invalid vector size")
	}
	for i := 0; i < rows; i++ {
		var norm float64
		for j := 0; j < dimensions; j++ {
			v := float64(math.Float32frombits(binary.LittleEndian.Uint32(data[(i*dimensions+j)*4:])))
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("non-finite vector")
			}
			norm += v * v
		}
		if math.Abs(math.Sqrt(norm)-1) > 0.001 {
			return fmt.Errorf("invalid vector norm")
		}
	}
	return nil
}

// VectorReader reads vector rows from an artifact.
type VectorReader struct {
	file             *os.File
	dimensions, rows int
}

// OpenVectors verifies the manifest, file size and every vector before opening rows.
func OpenVectors(dir string, index Index) (*VectorReader, error) {
	actual, err := ReadArtifact(dir)
	if err != nil {
		return nil, err
	}
	a, _ := json.Marshal(actual)
	b, _ := json.Marshal(index)
	if string(a) != string(b) {
		return nil, fmt.Errorf("embedding index mismatch")
	}
	f, err := os.Open(filepath.Join(dir, "vectors.f32"))
	if err != nil {
		return nil, fmt.Errorf("open vectors: %w", err)
	}
	fail := func(e error) (*VectorReader, error) { _ = f.Close(); return nil, e }
	info, err := f.Stat()
	if err != nil {
		return fail(fmt.Errorf("stat vectors: %w", err))
	}
	if info.Size() != int64(len(index.Rows))*int64(index.Dimensions)*4 {
		return fail(fmt.Errorf("invalid vector size"))
	}
	buf := make([]byte, index.Dimensions*4)
	for range index.Rows {
		if _, err = io.ReadFull(f, buf); err != nil {
			return fail(fmt.Errorf("read vector: %w", err))
		}
		if err = validateVectors(buf, 1, index.Dimensions); err != nil {
			return fail(err)
		}
	}
	return &VectorReader{f, index.Dimensions, len(index.Rows)}, nil
}

// Row returns a vector by its row number.
func (r *VectorReader) Row(i int) ([]float32, error) {
	if i < 0 || i >= r.rows {
		return nil, fmt.Errorf("vector row out of bounds")
	}
	buf := make([]byte, r.dimensions*4)
	if _, err := r.file.ReadAt(buf, int64(i)*int64(len(buf))); err != nil {
		return nil, fmt.Errorf("read vector row: %w", err)
	}
	if err := validateVectors(buf, 1, r.dimensions); err != nil {
		return nil, err
	}
	values := make([]float32, r.dimensions)
	for j := range values {
		values[j] = math.Float32frombits(binary.LittleEndian.Uint32(buf[j*4:]))
	}
	return values, nil
}

// Close closes the vector file.
func (r *VectorReader) Close() error { return r.file.Close() }
