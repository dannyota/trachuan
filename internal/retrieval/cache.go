package retrieval

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type DocumentCache interface {
	Get(release string, key [32]byte) ([]byte, bool)
	Put(release string, key [32]byte, value []byte)
}

type SequenceCache interface {
	MaxValueBytes() int64
	Get(release string, key [32]byte) ([]byte, bool)
	Put(release string, key [32]byte, value []byte)
}

type cacheContextKey struct{}
type requestCaches struct {
	documents  DocumentCache
	sequences  SequenceCache
	candidates CandidateCache
}

func WithCaches(ctx context.Context, documents DocumentCache, sequences SequenceCache, candidates CandidateCache) context.Context {
	return context.WithValue(ctx, cacheContextKey{}, requestCaches{documents: documents, sequences: sequences, candidates: candidates})
}

func (e *Engine) documentCache(ctx context.Context) DocumentCache {
	if caches, ok := ctx.Value(cacheContextKey{}).(requestCaches); ok {
		return caches.documents
	}
	return e.options.Documents
}
func (e *Engine) sequenceCache(ctx context.Context) SequenceCache {
	if caches, ok := ctx.Value(cacheContextKey{}).(requestCaches); ok {
		return caches.sequences
	}
	return e.options.Sequences
}

func documentKey(release, key string) [32]byte {
	raw, _ := json.Marshal([]string{release, key})
	return sha256.Sum256(raw)
}

func metadataJSON(doc DocumentEntry) []byte {
	type fields DocumentEntry
	raw, _ := json.Marshal(struct {
		*fields
		Status *Status `json:"status,omitempty"`
	}{fields: (*fields)(&doc)})
	return raw
}

func (e *Engine) cacheDocument(ctx context.Context, key string, doc DocumentEntry) {
	e.documentCache(ctx).Put(e.release.Rel, documentKey(e.release.Rel, key), metadataJSON(doc))
}

func (e *Engine) Documents(ctx context.Context, keys []string) (map[string]DocumentEntry, error) {
	cache := e.documentCache(ctx)
	if cache == nil {
		return e.loadDocuments(ctx, keys)
	}
	docs := make(map[string]DocumentEntry, len(keys))
	missing := make([]string, 0, len(keys))
	seen := make(map[string]bool, len(keys))
	for _, key := range keys {
		if seen[key] {
			continue
		}
		seen[key] = true
		raw, ok := cache.Get(e.release.Rel, documentKey(e.release.Rel, key))
		var doc DocumentEntry
		if ok && json.Unmarshal(raw, &doc) == nil {
			doc.Status = Status{}
			docs[key] = doc
		} else {
			missing = append(missing, key)
		}
	}
	if len(missing) == 0 {
		return docs, nil
	}
	loaded, err := e.loadDocuments(ctx, missing)
	if err != nil {
		return nil, err
	}
	for key, doc := range loaded {
		docs[key] = doc
		if ctx.Err() == nil {
			e.cacheDocument(ctx, key, doc)
		}
	}
	return docs, nil
}

type cachedSequence struct {
	Doc      string
	Document json.RawMessage `json:",omitempty"`
	Source   Document
	Units    []Unit
	Parts    map[int][]Unit
	Gaps     []Gap
	Offset   int
	More     bool
	Complete bool
}

func sequenceKey(release string, item ReadItem) [32]byte {
	mode := "provision"
	switch {
	case item.Relations:
		mode = "relations"
	case item.From != "":
		mode = "range"
	default:
		_, anchor, _ := ParseHandle(item.Handle)
		if anchor == "" {
			mode = "document"
		}
	}
	itemJSON, _ := json.Marshal(item)
	var canonical map[string]any
	_ = json.Unmarshal(itemJSON, &canonical)
	raw, _ := json.Marshal([]any{release, canonical, mode})
	return sha256.Sum256(raw)
}

func (e *Engine) sequence(ctx context.Context, tx pgx.Tx, c cursor) (readSequence, string, bool, error) {
	cache := e.sequenceCache(ctx)
	if cache != nil {
		if raw, ok := cache.Get(e.release.Rel, cursorSequenceKey(e.release.Rel, c)); ok {
			var stored cachedSequence
			if json.Unmarshal(raw, &stored) == nil && (!c.Item.Relations || c.Unit == stored.Offset) {
				seq := readSequence{doc: stored.Doc, source: stored.Source, units: stored.Units, parts: stored.Parts, gaps: stored.Gaps, offset: stored.Offset, more: stored.More, complete: stored.Complete}
				if len(stored.Document) > 0 {
					var doc DocumentEntry
					if json.Unmarshal(stored.Document, &doc) != nil {
						return e.uncachedSequence(ctx, tx, c)
					}
					seq.document = &doc
				}
				return seq, "", true, nil
			}
		}
	}
	return e.uncachedSequence(ctx, tx, c)
}

func (e *Engine) uncachedSequence(ctx context.Context, tx pgx.Tx, c cursor) (readSequence, string, bool, error) {
	seq, code, err := e.readScope(ctx, tx, c)
	return seq, code, false, err
}

func (e *Engine) saveSequence(ctx context.Context, c cursor, seq readSequence) func() {
	cache := e.sequenceCache(ctx)
	if cache == nil {
		return nil
	}
	return func() {
		if ctx.Err() != nil {
			return
		}
		limit := cache.MaxValueBytes()
		if limit <= 0 || sequenceSize(seq, limit) > limit || ctx.Err() != nil {
			return
		}
		stored := cachedSequence{Doc: seq.doc, Source: seq.source, Units: seq.units, Parts: seq.parts, Gaps: seq.gaps, Offset: seq.offset, More: seq.more, Complete: seq.complete}
		if seq.document != nil {
			stored.Document = metadataJSON(*seq.document)
		}
		raw, err := json.Marshal(stored)
		if err != nil || ctx.Err() != nil {
			return
		}
		cache.Put(e.release.Rel, cursorSequenceKey(e.release.Rel, c), raw)
	}
}

func (e *Engine) statusDocument(doc *DocumentEntry, source Document) {
	now := e.options.Now().In(time.FixedZone("Asia/Ho_Chi_Minh", 7*60*60))
	validity, _ := stamp(source, Arms{Date: Date{Year: now.Year(), Month: now.Month(), Day: now.Day()}})
	doc.Status = Status{Class: validity.StatusClass, Basis: validity.Basis, StatusBasis: validity.StatusBasis, Current: validity.Current}
}

func cursorSequenceKey(release string, c cursor) [32]byte {
	key := sequenceKey(release, c.Item)
	if c.PageCap == 0 && c.PartSize == 0 && !c.WholeDocument {
		return key
	}
	raw, _ := json.Marshal([]any{key, c.pageCap(), c.partSize(), c.WholeDocument})
	return sha256.Sum256(raw)
}
