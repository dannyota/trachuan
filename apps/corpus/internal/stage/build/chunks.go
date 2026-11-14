package build

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/embed"
)

func denseText(values []float32) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.FormatFloat(float64(v), 'g', -1, 32))
	}
	b.WriteByte(']')
	return b.String()
}
func (l *loader) checkChunkLineage(key string, file chunk.File) error {
	fetch, okFetch := l.items["fetch"][key]
	extract, okExtract := l.items["extract"][key]
	if !okFetch || !okExtract || fetch.ArtifactSHA256 == nil || extract.ArtifactSHA256 == nil || file.FetchSHA256 != *fetch.ArtifactSHA256 || file.ExtractSHA256 != *extract.ArtifactSHA256 {
		return fmt.Errorf("chunk lineage mismatch")
	}
	return nil
}
func (d *loadedDocument) owner(c chunk.Chunk) (int64, error) {
	id, ok := d.nodes[c.NodeID]
	if !ok || d.anchors[c.NodeID] != c.Anchor {
		return 0, fmt.Errorf("chunk provision identity mismatch")
	}
	return id, nil
}
func (l *loader) setState(ctx context.Context, id int64, state string) error {
	_, err := l.tx.Exec(ctx, `UPDATE document SET content_state=$2 WHERE id=$1`, id, state)
	if err != nil {
		return fmt.Errorf("update content state: %w", err)
	}
	return nil
}
func (l *loader) loadChunks(ctx context.Context) error {
	for _, key := range l.inv.ExpectedKeys {
		doc := l.docs[key]
		if doc == nil || len(doc.nodes) == 0 || doc.chunk == nil {
			continue
		}
		fetch := l.items["fetch"][key]
		extract := l.items["extract"][key]
		if fetch.ArtifactSHA256 == nil || extract.ArtifactSHA256 == nil {
			return fmt.Errorf("missing chunk lineage")
		}
		expected := stage.RecipeHash("chunk-input", "1", key, *fetch.ArtifactSHA256, *extract.ArtifactSHA256)
		if err := checkInputHash(*doc.chunk, expected); err != nil {
			return err
		}
		dir, err := artifactDir(l.artifacts, *doc.chunk)
		if err != nil {
			return fmt.Errorf("chunk artifact: %w", err)
		}
		file, coverage, err := chunk.ReadArtifact(dir)
		if err != nil {
			return fmt.Errorf("read chunks: %w", err)
		}
		if file.Version != 1 || file.DocumentKey != key || coverage.Version != 1 || coverage.DocumentKey != key {
			return fmt.Errorf("chunk identity mismatch")
		}
		if err = l.checkChunkLineage(key, file); err != nil {
			return err
		}
		for _, o := range coverage.Omissions {
			if o.Reason == "listing_only" {
				continue
			}
			l.addGap("build", key, "chunk_omission", fmt.Sprintf("%s:%d-%d:%s", o.NodeID, o.Start, o.End, o.Reason))
		}
		if len(file.Chunks) == 0 {
			if err = l.setState(ctx, doc.id, "no_chunks"); err != nil {
				return err
			}
			continue
		}
		if doc.embedding == nil {
			l.counts.UnavailableArtifacts++
			if err = l.setState(ctx, doc.id, "embedding_gap"); err != nil {
				return err
			}
			continue
		}
		if err = checkInputHash(*doc.embedding, *doc.chunk.ArtifactSHA256); err != nil {
			return err
		}
		embedDir, err := artifactDir(l.artifacts, *doc.embedding)
		if err != nil {
			return fmt.Errorf("embed artifact: %w", err)
		}
		idx, err := embed.ReadArtifact(embedDir)
		if err != nil {
			return fmt.Errorf("read embeddings: %w", err)
		}
		if idx.DocumentKey != key || idx.ChunkSHA256 != *doc.chunk.ArtifactSHA256 || idx.Recipe != l.recipes.Dense {
			return fmt.Errorf("embedding identity mismatch")
		}
		if err = checkVectorSize(filepath.Join(embedDir, "vectors.f32"), len(idx.Rows), idx.Dimensions); err != nil {
			return err
		}
		if err = reconcile(file, idx); err != nil {
			return err
		}
		l.counts.Rejected += int64(len(idx.Rejected))
		if len(idx.Rows) == 0 {
			if err = l.setState(ctx, doc.id, "embedding_gap"); err != nil {
				return err
			}
			continue
		}
		vr, err := embed.OpenVectors(embedDir, idx)
		if err != nil {
			return fmt.Errorf("open vectors: %w", err)
		}
		byOrdinal := make(map[int]chunk.Chunk, len(file.Chunks))
		for _, c := range file.Chunks {
			byOrdinal[c.Ordinal] = c
		}
		for _, row := range idx.Rows {
			c := byOrdinal[row.ChunkOrdinal]
			input := strings.TrimSpace(c.ContextPrefix) + "\n" + strings.TrimSpace(c.Content)
			hash := fmt.Sprintf("%x", sha256.Sum256([]byte(input)))
			if hash != row.InputSHA256 {
				_ = vr.Close()
				return fmt.Errorf("embedding input hash mismatch")
			}
			pid, ownerErr := doc.owner(c)
			if ownerErr != nil {
				_ = vr.Close()
				return fmt.Errorf("chunk provision missing")
			}
			if l.counts.Chunks >= 500000 {
				_ = vr.Close()
				return fmt.Errorf("chunk limit exceeded")
			}
			vector, e := vr.Row(row.Row)
			if e != nil {
				_ = vr.Close()
				return fmt.Errorf("read vector: %w", e)
			}
			spans, e := json.Marshal(c.Spans)
			if e != nil {
				_ = vr.Close()
				return fmt.Errorf("encode spans: %w", e)
			}
			_, e = l.tx.Exec(ctx, `INSERT INTO build_chunk(id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::vector)`, l.counts.Chunks+1, doc.id, pid, c.Ordinal, c.ContextPrefix, c.Content, spans, row.InputSHA256, row.TokenCount, denseText(vector))
			if e != nil {
				_ = vr.Close()
				return fmt.Errorf("insert chunk: %w", e)
			}
			l.counts.Chunks++
			l.counts.DenseVectors++
		}
		if err = vr.Close(); err != nil {
			return fmt.Errorf("close vectors: %w", err)
		}
	}
	return nil
}
