package embed

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"github.com/jackc/pgx/v5"
)

type pending struct {
	ordinal int
	text    string
	count   int
}

func hashText(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
func requestSize(hash string, texts []string) int {
	b, _ := json.Marshal(struct {
		RecipeHash string   `json:"recipe_hash"`
		Texts      []string `json:"texts"`
	}{hash, texts})
	return len(b)
}
func (s *Stage) build(ctx context.Context, file chunk.File, _ chunk.Coverage, hash string) (Index, []byte, error) {
	idx := Index{Version: 1, DocumentKey: file.DocumentKey, ChunkSHA256: hash, RecipeHash: s.expected.Hash(), Recipe: s.expected, Dimensions: s.expected.Dimensions, Rows: []Row{}, Rejected: []Rejected{}}
	var batch []pending
	var vectors []byte
	last := -1
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		texts := make([]string, len(batch))
		for i, p := range batch {
			texts[i] = p.text
		}
		counts, err := s.client.Tokenize(ctx, idx.RecipeHash, texts)
		if err != nil {
			return classify(err)
		}
		var accepted []pending
		var acceptedText []string
		for i, p := range batch {
			p.count = counts[i]
			if p.count > s.expected.TokenCap {
				n := p.count
				idx.Rejected = append(idx.Rejected, Rejected{p.ordinal, &n, "embedding_input_too_long"})
			} else {
				accepted = append(accepted, p)
				acceptedText = append(acceptedText, p.text)
			}
		}
		if len(accepted) > 0 {
			vs, e := s.client.Embed(ctx, idx.RecipeHash, acceptedText)
			if e != nil {
				return classify(e)
			}
			for i, v := range vs {
				if len(v.Values) != idx.Dimensions || v.TokenCount != accepted[i].count || v.RecipeHash != idx.RecipeHash {
					return fmt.Errorf("invalid embedding response")
				}
				start := len(vectors)
				for _, value := range v.Values {
					vectors = binary.LittleEndian.AppendUint32(vectors, math.Float32bits(value))
				}
				if e = validateVectors(vectors[start:], 1, idx.Dimensions); e != nil {
					return e
				}
				idx.Rows = append(idx.Rows, Row{len(idx.Rows), accepted[i].ordinal, accepted[i].count, hashText(accepted[i].text)})
			}
		}
		batch = nil
		return nil
	}
	for _, c := range file.Chunks {
		if c.Ordinal <= last {
			return Index{}, nil, fmt.Errorf("chunk ordinals are not increasing")
		}
		last = c.Ordinal
		text := strings.TrimSpace(c.ContextPrefix) + "\n" + strings.TrimSpace(c.Content)
		if requestSize(idx.RecipeHash, []string{text}) > 1048576 {
			if err := flush(); err != nil {
				return Index{}, nil, err
			}
			idx.Rejected = append(idx.Rejected, Rejected{c.Ordinal, nil, "embedding_input_bytes_exceeded"})
			continue
		}
		texts := make([]string, 0, len(batch)+1)
		for _, p := range batch {
			texts = append(texts, p.text)
		}
		texts = append(texts, text)
		if len(texts) > 16 || requestSize(idx.RecipeHash, texts) > 1048576 {
			if err := flush(); err != nil {
				return Index{}, nil, err
			}
		}
		batch = append(batch, pending{ordinal: c.Ordinal, text: text})
	}
	if err := flush(); err != nil {
		return Index{}, nil, err
	}
	return idx, vectors, nil
}

// Rows does not create workshop rows for vectors.
func (*Stage) Rows(context.Context, stage.Env, artifact.Result) (func(context.Context, pgx.Tx, int64) error, error) {
	return nil, nil
}

// Resolve recreates the gaps from a verified artifact.
func (*Stage) Resolve(_ context.Context, env stage.Env, _ stage.Run, _ stage.Item, art artifact.Result) (stage.Resolution, error) {
	dir := env.Artifacts.Abs(art.Path)
	hash, err := artifact.Verify(dir)
	if err != nil {
		return stage.Resolution{}, fmt.Errorf("verify embed artifact: %w", err)
	}
	if hash != art.SHA256 {
		return stage.Resolution{}, fmt.Errorf("embed artifact hash mismatch")
	}
	idx, err := ReadArtifact(dir)
	if err != nil {
		return stage.Resolution{}, err
	}
	report, err := readReport(dir)
	if err != nil {
		return stage.Resolution{}, err
	}
	reader, err := OpenVectors(dir, idx)
	if err != nil {
		return stage.Resolution{}, err
	}
	if err = reader.Close(); err != nil {
		return stage.Resolution{}, err
	}
	if report.Rows != len(idx.Rows) || report.Rejected != len(idx.Rejected) {
		return stage.Resolution{}, fmt.Errorf("embedding report counts mismatch")
	}
	gaps := make([]stage.Gap, 0, len(report.Gaps))
	for _, gap := range report.Gaps {
		gaps = append(gaps, stage.Gap{Kind: gap.Kind, Detail: gap.Detail})
	}
	return stage.Resolution{Gaps: gaps}, nil
}
