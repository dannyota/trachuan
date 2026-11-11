package cli

import (
	"context"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/ocrstore"
	"github.com/jackc/pgx/v5/pgtype"
)

const importTextLimit = 50_000_000

type ocrImporter interface {
	Put(context.Context, string, string, string, pgtype.Int4, []byte) (bool, error)
	Lookup(context.Context, []string, []string, string) (map[string]ocrstore.Entry, error)
}

type importCounts struct{ imported, present, notInRun, rejected int }

func importBanhmi(ctx context.Context, s ocrImporter, dir string, pdfs map[string]bool) (importCounts, error) {
	var counts importCounts
	root := filepath.Join(dir, "ocr")
	entries, err := os.ReadDir(root)
	if err != nil {
		return counts, fmt.Errorf("read OCR import directory: %w", err)
	}
	hashes := make([]string, 0, len(pdfs))
	for hash := range pdfs {
		hashes = append(hashes, hash)
	}
	existing, err := s.Lookup(ctx, hashes, []string{"vision-ocr/builtin-latest"}, "")
	if err != nil {
		return counts, fmt.Errorf("look up OCR entries: %w", err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return counts, fmt.Errorf("import OCR: %w", err)
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".txt") || len(name) != 68 || strings.ToLower(name) != name {
			counts.rejected++
			continue
		}
		hash := strings.TrimSuffix(name, ".txt")
		if _, err := hex.DecodeString(hash); err != nil {
			counts.rejected++
			continue
		}
		if !pdfs[hash] {
			counts.notInRun++
			continue
		}
		path := filepath.Join(root, name)
		info, err := os.Lstat(path)
		if err != nil {
			return counts, fmt.Errorf("stat OCR import: %w", err)
		}
		if !info.Mode().IsRegular() {
			counts.rejected++
			continue
		}
		if _, ok := existing[hash]; ok {
			counts.present++
			continue
		}
		if info.Size() <= 0 || info.Size() > importTextLimit {
			counts.rejected++
			continue
		}
		f, err := os.Open(path)
		if err != nil {
			return counts, fmt.Errorf("open OCR import: %w", err)
		}
		data, readErr := io.ReadAll(io.LimitReader(f, importTextLimit+1))
		closeErr := f.Close()
		if readErr != nil {
			return counts, fmt.Errorf("read OCR import: %w", readErr)
		}
		if closeErr != nil {
			return counts, fmt.Errorf("close OCR import: %w", closeErr)
		}
		if len(data) == 0 || len(data) > importTextLimit || !utf8.Valid(data) {
			counts.rejected++
			continue
		}
		present, err := s.Put(ctx, hash, "vision-ocr/builtin-latest", "banhmi", pgtype.Int4{}, data)
		if err != nil {
			return counts, fmt.Errorf("store imported OCR: %w", err)
		}
		if present {
			counts.present++
		} else {
			counts.imported++
		}
	}
	return counts, nil
}
