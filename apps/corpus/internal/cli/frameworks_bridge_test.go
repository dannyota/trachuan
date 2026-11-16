package cli

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	lawembed "danny.vn/trachuan/apps/corpus/internal/stage/embed"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/frameworks/build"
)

func TestFrameworksBridge(t *testing.T) {
	if os.Getenv("FRAMEWORKS_TEST_BRIDGE_MODE") != "1" {
		return
	}
	var req struct {
		Op, Dir, Key, Hash string
		Chunks             []build.Chunk
		Reject             map[int]string
	}
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		os.Exit(2)
	}
	startFrameworkEmbedder(t)
	service := newFrameworkEmbedder()
	if req.Op == "read" {
		rows, err := service.Read(context.Background(), req.Dir, req.Key, req.Chunks, req.Hash)
		if err != nil || json.NewEncoder(os.Stdout).Encode(rows) != nil {
			os.Exit(10)
		}
		os.Exit(0)
	}
	if req.Op == "validate" {
		if service.Validate(context.Background(), req.Dir, req.Key, req.Chunks, req.Hash) != nil {
			os.Exit(3)
		}
		os.Exit(0)
	}
	if err := service.Check(context.Background()); err != nil {
		os.Exit(4)
	}
	result, err := service.Build(context.Background(), req.Key, req.Chunks, req.Hash)
	if err != nil {
		os.Exit(5)
	}
	if len(req.Reject) != 0 {
		var idx lawembed.Index
		if json.Unmarshal(result.Index, &idx) != nil {
			os.Exit(6)
		}
		vectors := result.Vectors
		result.Vectors = nil
		original := idx.Rows
		idx.Rows = []lawembed.Row{}
		for _, row := range original {
			if kind, ok := req.Reject[row.ChunkOrdinal]; ok {
				var count *int
				if kind == "embedding_input_too_long" {
					n := embedding.DocumentRecipe1().TokenCap + 1
					count = &n
				}
				idx.Rejected = append(idx.Rejected, lawembed.Rejected{ChunkOrdinal: row.ChunkOrdinal, TokenCount: count, Kind: kind})
				result.Rejected = append(result.Rejected, build.EmbedRejection{Ordinal: row.ChunkOrdinal, TokenCount: count, Kind: kind})
				continue
			}
			offset := row.Row * idx.Dimensions * 4
			if offset < 0 || offset+idx.Dimensions*4 > len(vectors) {
				os.Exit(7)
			}
			result.Vectors = append(result.Vectors, vectors[offset:offset+idx.Dimensions*4]...)
			row.Row = len(idx.Rows)
			idx.Rows = append(idx.Rows, row)
		}
		result.Rows = len(idx.Rows)
		result.Index, err = json.Marshal(idx)
		if err != nil {
			os.Exit(8)
		}
	}
	if json.NewEncoder(os.Stdout).Encode(result) != nil {
		os.Exit(9)
	}
	os.Exit(0)
}
