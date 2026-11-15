package build

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"danny.vn/trachuan/internal/textsplit"
)

func chunkStep(ctx context.Context, base, dir string) error {
	reg, err := readRegistry(filepath.Join(base, "snapshot/sources/frameworks.yaml"))
	if err != nil {
		return err
	}
	controls, err := loadRows[Control](filepath.Join(base, "parse/controls.jsonl"))
	if err != nil {
		return err
	}
	var chunks []Chunk
	for _, v := range releaseVersions(reg) {
		if err := ctx.Err(); err != nil {
			return err
		}
		var nodes []Control
		byCitation := map[string]Control{}
		for _, c := range controls {
			if c.Code == v.Code && c.Version == v.Version {
				nodes = append(nodes, c)
				byCitation[c.Citation] = c
			}
		}
		sort.Slice(nodes, func(i, j int) bool { return nodes[i].Ordinal < nodes[j].Ordinal })
		bodyOrdinal := 0
		for ordinal, c := range nodes {
			path, err := citationPath(c, byCitation)
			if err != nil {
				return err
			}
			prefix := []string{v.Name + " " + v.Version, path}
			public := []string{c.Citation, c.Kind}
			if c.Title != nil {
				prefix = append(prefix, *c.Title)
				public = append(public, *c.Title)
			}
			chunks = append(chunks, Chunk{Index: "public", DocumentKey: "fwp:" + v.Code + ":" + v.Version, Ordinal: ordinal, Citation: c.Citation, Content: strings.Join(public, "\n")})
			body := []rune(c.Body)
			for index, event := range textsplit.Split(body, 0, 512) {
				var pieces []string
				for _, r := range event.Ranges {
					pieces = append(pieces, string(body[r.Start:r.End]))
				}
				content := strings.Join(pieces, string(event.Separator))
				chunks = append(chunks, Chunk{Index: "body", DocumentKey: "fw:" + v.Code + ":" + v.Version, Ordinal: bodyOrdinal, Citation: c.Citation, ChunkIndex: index, ContextPrefix: strings.Join(prefix, "\n"), Content: content})
				bodyOrdinal++
			}
		}
	}
	sort.Slice(chunks, func(i, j int) bool {
		if chunks[i].DocumentKey != chunks[j].DocumentKey {
			return chunks[i].DocumentKey < chunks[j].DocumentKey
		}
		return chunks[i].Ordinal < chunks[j].Ordinal
	})
	if err := writeRows(dir, "chunks.jsonl", chunks); err != nil {
		return err
	}
	return writeGaps(dir, nil)
}

func citationPath(c Control, nodes map[string]Control) (string, error) {
	path := []string{c.Citation}
	seen := map[string]bool{c.Citation: true}
	for c.Parent != nil {
		parent, exists := nodes[*c.Parent]
		if !exists || seen[parent.Citation] {
			return "", fmt.Errorf("invalid control lineage")
		}
		seen[parent.Citation] = true
		path = append(path, parent.Citation)
		c = parent
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return strings.Join(path, " > "), nil
}
