package extract

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/textparse"
	"danny.vn/trachuan/apps/corpus/internal/vbpl"
)

type ocrSource interface{ available(string) bool }
type absentOCR struct{}

func (absentOCR) available(string) bool { return false }

type selection struct {
	source, gap, name, hash string
	data                    []byte
	tooLarge, parseLimit    bool
}

// InputHash binds the fetch manifest and the selected text source.
func InputHash(fetchHash, source string) string { return fetch.ExtractInputHash(fetchHash, source) }

func selectionHash(hash, source string) string { return InputHash(hash, source) }

func selectFiles(dir, id string, files []fetch.DocumentFile, ocr ocrSource) (selection, error) {
	for _, f := range files {
		if f.Name != id+"_content.html" {
			continue
		}
		if len(f.SHA256) != 64 {
			return selection{}, fmt.Errorf("invalid transcription hash")
		}
		if _, err := hex.DecodeString(f.SHA256); err != nil {
			return selection{}, fmt.Errorf("invalid transcription hash: %w", err)
		}
		info, err := os.Lstat(filepath.Join(dir, "files", f.SHA256))
		if err != nil {
			return selection{}, fmt.Errorf("stat transcription: %w", err)
		}
		if !info.Mode().IsRegular() {
			return selection{}, fmt.Errorf("transcription is not a regular file")
		}
		if f.Size > 50_000_000 || info.Size() > 50_000_000 {
			return selection{source: "html_transcription", name: f.Name, hash: f.SHA256, tooLarge: true}, nil
		}
		data, err := os.ReadFile(filepath.Join(dir, "files", f.SHA256))
		if err != nil {
			return selection{}, fmt.Errorf("read transcription: %w", err)
		}
		paragraphs, err := textparse.ParagraphsFromHTML(data)
		var limit *textparse.LimitError
		if err != nil && !errors.As(err, &limit) {
			return selection{}, fmt.Errorf("read transcription paragraphs: %w", err)
		}
		count := 0
		for _, p := range paragraphs {
			count += utf8.RuneCountInString(p.Text)
		}
		if count >= 300 || limit != nil {
			return selection{source: "html_transcription", name: f.Name, hash: f.SHA256, data: data, parseLimit: limit != nil}, nil
		}
	}
	pdfs := map[string]bool{}
	usable := 0
	unsupported := false
	for _, f := range files {
		ext := strings.ToLower(filepath.Ext(f.Name))
		switch ext {
		case ".pdf":
			pdfs[f.SHA256] = true
		case ".doc", ".rar", ".zip", ".7z", ".tar", ".gz":
			unsupported = true
		}
	}
	for hash := range pdfs {
		if ocr.available(hash) {
			usable++
		}
	}
	gap := "no_text_source"
	switch {
	case len(pdfs) == 1 || usable == 1:
		gap = "needs_ocr"
	case len(pdfs) > 1:
		gap = "text_source_ambiguous"
	case unsupported:
		gap = "file_format_unsupported"
	}
	return selection{source: "none", gap: gap}, nil
}
func choose(ctx context.Context, dir, key string) (selection, []vbpl.Node, error) {
	parts := strings.SplitN(key, ":", 3)
	if len(parts) != 3 {
		return selection{}, nil, fmt.Errorf("invalid document key")
	}
	if parts[1] == "vbpl" {
		b, err := os.ReadFile(filepath.Join(dir, "tree.json"))
		if err != nil {
			return selection{}, nil, fmt.Errorf("read tree: %w", err)
		}
		nodes, err := vbpl.DecodeTree(ctx, strings.NewReader(string(b)), vbpl.MaxTreeDepth, vbpl.MaxTreeNodes)
		if err != nil {
			return selection{}, nil, err
		}
		if len(nodes) > 0 {
			rows, _, extractErr := extractTree(ctx, nodes)
			if extractErr != nil {
				return selection{}, nil, extractErr
			}
			if !headingOnly(rows) {
				return selection{source: "source_tree"}, nodes, nil
			}
			doc, readErr := fetch.ReadDocument(dir, key)
			if readErr != nil {
				return selection{}, nil, fmt.Errorf("read fetch document: %w", readErr)
			}
			selected, selectErr := selectFiles(dir, parts[2], doc.Files, absentOCR{})
			if selectErr != nil {
				return selection{}, nil, selectErr
			}
			if selected.source != "none" && !selected.tooLarge && !selected.parseLimit {
				return selected, nil, nil
			}
			return selection{source: "source_tree", gap: "tree_headings_only"}, nodes, nil
		}
	}
	doc, err := fetch.ReadDocument(dir, key)
	if err != nil {
		return selection{}, nil, fmt.Errorf("read fetch document: %w", err)
	}
	s, err := selectFiles(dir, parts[2], doc.Files, absentOCR{})
	return s, nil, err
}

func headingOnly(rows []provision) bool {
	children := make(map[string]bool)
	for _, row := range rows {
		if row.ParentNodeID != nil {
			children[*row.ParentNodeID] = true
		}
	}
	found := false
	for _, row := range rows {
		if row.Kind == "dieu" {
			found = true
		}
		if row.Kind == "dieu" && children[row.NodeID] {
			return false
		}
		text := strings.Join(strings.Fields(row.Text), " ")
		label := strings.Join(strings.Fields(row.Label), " ")
		heading := strings.Join(strings.Fields(row.Heading), " ")
		if text == "" && row.Kind != "dieu" {
			continue
		}
		if text != label && text != label+"." && text != label+" "+heading && text != label+". "+heading {
			return false
		}
		if heading != "" && (text == label || text == label+".") {
			return false
		}
	}
	return found
}
