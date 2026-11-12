package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"danny.vn/trachuan/apps/corpus/internal/ocrvision"
	"danny.vn/trachuan/apps/corpus/internal/stage/extract"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
)

func (e *checkRun) visionDocuments(ctx context.Context, pdf string) ([]ocrvision.Document, error) {
	q := store.New(e.db)
	active, err := membership.FinalActiveKeys(ctx, q, e.run)
	if err != nil {
		return nil, err
	}
	selected, err := q.ListVisionSelectionKeys(ctx, e.run.ID)
	if err != nil {
		return nil, fmt.Errorf("list frozen OCR gaps: %w", err)
	}
	keys := make(map[string]bool, len(selected))
	for _, key := range selected {
		keys[key] = true
	}
	inputs, err := q.ListExtractInputs(ctx, e.run.ID)
	if err != nil {
		return nil, fmt.Errorf("list Vision inputs: %w", err)
	}
	docs := make(map[string]ocrvision.Document)
	for _, input := range inputs {
		// An explicit comparison may name any PDF in the run, including inactive keys.
		if pdf == "" && (!keys[input.ItemKey] || (active != nil && !active[input.ItemKey])) {
			continue
		}
		item, getErr := q.GetExtractInput(ctx, store.GetExtractInputParams{RunID: e.run.ID, ItemKey: input.ItemKey})
		if getErr != nil {
			return nil, fmt.Errorf("load Vision input: %w", getErr)
		}
		dir := e.artifacts.Abs(item.Path)
		hash, verifyErr := verifyCheckBundle(dir, input.ItemKey)
		if verifyErr != nil {
			return nil, fmt.Errorf("verify Vision bundle: %w", verifyErr)
		}
		if hash != item.Sha256 || hash != input.Sha256 {
			return nil, fmt.Errorf("vision bundle hash mismatch")
		}
		doc, readErr := fetch.ReadDocument(dir, input.ItemKey)
		if readErr != nil {
			return nil, fmt.Errorf("read Vision bundle: %w", readErr)
		}
		// Gap rows do not freeze a PDF hash. Recover the single distinct PDF from
		// the same verified fetch inputs used by the check command.
		for _, hash := range visionFiles(doc.Files, pdf) {
			if _, exists := docs[hash]; !exists {
				docs[hash] = ocrvision.Document{Hash: hash, Path: filepath.Join(dir, "files", hash)}
			}
		}
	}
	if pdf != "" && len(docs) == 0 {
		return nil, fmt.Errorf("--pdf is not listed in a verified fetch bundle of this run")
	}
	ordered := make([]ocrvision.Document, 0, len(docs))
	for _, doc := range docs {
		ordered = append(ordered, doc)
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Hash < ordered[j].Hash })
	return filterVisionEntries(ctx, ocrstore.Open(e.root, e.db), ordered, pdf != "")
}

func visionFiles(files []fetch.DocumentFile, pdf string) []string {
	hashes := map[string]bool{}
	for _, file := range files {
		if strings.EqualFold(filepath.Ext(file.Name), ".pdf") && (pdf == "" || pdf == file.SHA256) {
			hashes[file.SHA256] = true
		}
	}
	if len(hashes) != 1 {
		return nil
	}
	for hash := range hashes {
		return []string{hash}
	}
	return nil
}

type visionEntries interface {
	Lookup(context.Context, []string, []string, string) (map[string]ocrstore.Entry, error)
}

func filterVisionEntries(ctx context.Context, source visionEntries, docs []ocrvision.Document, explicit bool) ([]ocrvision.Document, error) {
	engines := extract.AcceptedEngines()
	if explicit {
		engines = []string{ocrvision.EngineID}
	}
	hashes := make([]string, 0, len(docs))
	for _, doc := range docs {
		hashes = append(hashes, doc.Hash)
	}
	entries, err := source.Lookup(ctx, hashes, engines, "")
	if err != nil {
		return nil, fmt.Errorf("look up Vision entries: %w", err)
	}
	filtered := make([]ocrvision.Document, 0, len(docs))
	for _, doc := range docs {
		_, present := entries[doc.Hash]
		if present && !explicit {
			continue
		}
		doc.Present = present
		filtered = append(filtered, doc)
	}
	return filtered, nil
}
