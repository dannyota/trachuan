package extract

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/textparse"
	"golang.org/x/text/unicode/norm"
)

func frozenMismatch(dir, key string) (selection, error) {
	if reason := manualMismatches()[key]; reason != "" {
		return selection{source: "none", gap: "text_number_mismatch", reason: reason, consulted: true}, nil
	}
	parts := strings.SplitN(key, ":", 3)
	if len(parts) != 3 || parts[1] != "vbpl" {
		return selection{}, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, "files.json"))
	if errors.Is(err, os.ErrNotExist) {
		return selection{}, nil
	}
	if err != nil {
		return selection{}, fmt.Errorf("read file list: %w", err)
	}
	var files []fetch.DocumentFile
	if err = json.Unmarshal(data, &files); err != nil {
		return selection{}, fmt.Errorf("decode file list: %w", err)
	}
	found := false
	for _, file := range files {
		if file.Name == parts[2]+"_content.html" {
			found = true
			break
		}
	}
	if !found {
		return selection{}, nil
	}
	doc, err := fetch.ReadDocument(dir, key)
	if err != nil {
		return selection{}, fmt.Errorf("read fetch document: %w", err)
	}
	reason, err := transcriptionMismatch(dir, parts[2], doc)
	if err != nil {
		return selection{}, err
	}
	if reason == "" {
		return selection{}, nil
	}
	return selection{source: "none", gap: "text_number_mismatch", reason: reason, consulted: true}, nil
}

func transcriptionMismatch(dir, id string, doc fetch.Document) (string, error) {
	if doc.IsConsolidated || strings.Contains(strings.ToUpper(doc.Number), "VBHN") {
		return "", nil
	}
	for _, f := range doc.Files {
		if f.Name != id+"_content.html" {
			continue
		}
		if len(f.SHA256) != 64 {
			return "", fmt.Errorf("invalid transcription hash")
		}
		if _, err := hex.DecodeString(f.SHA256); err != nil {
			return "", fmt.Errorf("invalid transcription hash: %w", err)
		}
		path := filepath.Join(dir, "files", f.SHA256)
		info, err := os.Lstat(path)
		if err != nil {
			return "", fmt.Errorf("stat transcription: %w", err)
		}
		if !info.Mode().IsRegular() {
			return "", fmt.Errorf("transcription is not a regular file")
		}
		if f.Size > 50_000_000 || info.Size() > 50_000_000 {
			return "", nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("read transcription: %w", err)
		}
		paragraphs, err := textparse.ParagraphsFromHTML(data)
		var limit *textparse.LimitError
		if errors.As(err, &limit) {
			return "", nil
		}
		if err != nil {
			return "", fmt.Errorf("read transcription paragraphs: %w", err)
		}
		var blocks []string
		for _, p := range paragraphs {
			blocks = append(blocks, p.Text)
		}
		header := strings.Join(blocks, " ")
		for _, marker := range []string{"Căn cứ", "CĂN CỨ", "Chiểu", "CHIỂU", "Theo đề nghị", "Xét đề nghị", "Điều 1"} {
			if i := strings.Index(header, marker); i >= 0 {
				header = header[:i]
			}
		}
		matches := regexp.MustCompile(`(?:Số\s*:\s*|SỐ\s+)([0-9]+[^\s]*)`).FindAllStringSubmatch(header, -1)
		digits := func(s string) string {
			i := 0
			for i < len(s) && s[i] >= '0' && s[i] <= '9' {
				i++
			}
			if i == 0 {
				return ""
			}
			n := strings.TrimLeft(s[:i], "0")
			if n == "" {
				return "0"
			}
			return n
		}
		for _, match := range matches {
			token := match[1]
			if !strings.ContainsAny(token, "/-") || digits(token) == digits(doc.Number) || strings.Contains(normalizeNumber(doc.Title), normalizeNumber(token)) {
				continue
			}
			if subjectMismatch(header, doc.Title) {
				return fmt.Sprintf("header %s metadata %s", token, doc.Number), nil
			}
		}
		return "", nil
	}
	return "", nil
}

func normalizeNumber(s string) string {
	s = strings.ToUpper(norm.NFC.String(s))
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) || r == '.' {
			return -1
		}
		if r == '-' {
			return '/'
		}
		return r
	}, s)
}
