package build

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"danny.vn/trachuan/internal/frameworks/relate"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

type posting struct {
	token string
	tf    int
}

func lexicalPostings(content, prefix string) ([]posting, int, error) {
	tokens, err := lexical.Tokenize(content+" "+prefix, lexical.Recipe1())
	if err != nil {
		return nil, 0, hide("encode lexical postings", err)
	}
	counts := map[string]int{}
	for _, token := range tokens {
		counts[token]++
	}
	terms := make([]string, 0, len(counts))
	for term := range counts {
		terms = append(terms, term)
	}
	sort.Strings(terms)
	rows := make([]posting, 0, len(terms))
	for _, term := range terms {
		rows = append(rows, posting{term, counts[term]})
	}
	return rows, len(tokens), nil
}

func loadChunks(ctx context.Context, tx pgx.Tx, base string, d databaseInputs, ids databaseIDs, embedder Embedder) error {
	groups := map[string][]Chunk{}
	versions := map[string]int64{}
	for _, v := range d.versions {
		for _, prefix := range []string{"fw:", "fwp:"} {
			key := prefix + versionKey(v.Code, v.Version)
			groups[key] = nil
			versions[key] = ids.versions[versionKey(v.Code, v.Version)]
		}
	}
	for _, c := range d.chunks {
		if _, ok := groups[c.DocumentKey]; !ok {
			return fmt.Errorf("unknown chunk document")
		}
		groups[c.DocumentKey] = append(groups[c.DocumentKey], c)
	}
	keys := make([]string, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	bodyID, publicID := int64(0), int64(0)
	hash, err := readFile(filepath.Join(base, "chunk/manifest.json"))
	if err != nil {
		return err
	}
	for _, key := range keys {
		chunks := groups[key]
		vectors, err := embedder.Read(ctx, filepath.Join(base, "embed/artifacts", digest([]byte(key))), key, chunks, digest(hash))
		if err != nil {
			return hide("read embedded chunks", err)
		}
		byOrdinal := map[int]Chunk{}
		for _, c := range chunks {
			if _, ok := byOrdinal[c.Ordinal]; ok {
				return fmt.Errorf("duplicate chunk ordinal")
			}
			byOrdinal[c.Ordinal] = c
		}
		for _, v := range vectors {
			c, ok := byOrdinal[v.Ordinal]
			if !ok {
				return fmt.Errorf("unknown embedded chunk")
			}
			delete(byOrdinal, v.Ordinal)
			public := strings.HasPrefix(key, "fwp:")
			index := "body"
			if public {
				index = "public"
			}
			if c.Index != index {
				return fmt.Errorf("chunk index mismatch")
			}
			parts := strings.SplitN(key, ":", 3)
			control, err := requiredID(ids.controls, relate.Key{Code: parts[1], Version: parts[2], Citation: c.Citation})
			if err != nil {
				return err
			}
			id := &bodyID
			if public {
				id = &publicID
			}
			*id++
			if err = insertChunk(ctx, tx, index, *id, versions[key], control, c, v); err != nil {
				return err
			}
		}
	}
	return nil
}

func insertChunk(ctx context.Context, tx pgx.Tx, index string, id, version, control int64, c Chunk, v Vector) error {
	prefix := c.ContextPrefix
	if index == "public" {
		if prefix != "" {
			return fmt.Errorf("public chunk has a context prefix")
		}
	}
	postings, length, err := lexicalPostings(c.Content, prefix)
	if err != nil {
		return err
	}
	vector, err := json.Marshal(v.Values)
	if err != nil {
		return hide("encode dense vector", err)
	}
	if index == "body" {
		_, err = tx.Exec(ctx, `INSERT INTO body_chunk(id,version_id,control_id,ordinal,content,input_sha256,token_count,embedding) VALUES($1,$2,$3,$4,$5,$6,$7,$8::vector)`, id, version, control, c.ChunkIndex, c.Content, v.InputSHA256, length, string(vector))
	} else {
		_, err = tx.Exec(ctx, `INSERT INTO public_chunk(id,version_id,control_id,content,input_sha256,token_count,embedding) VALUES($1,$2,$3,$4,$5,$6,$7::vector)`, id, version, control, c.Content, v.InputSHA256, length, string(vector))
	}
	if err != nil {
		return hide("load indexed chunk", err)
	}
	for _, p := range postings {
		if _, err = tx.Exec(ctx, `INSERT INTO `+index+`_posting(version_id,chunk_id,token,tf) VALUES($1,$2,$3,$4)`, version, id, p.token, p.tf); err != nil {
			return hide("load lexical posting", err)
		}
	}
	return nil
}
