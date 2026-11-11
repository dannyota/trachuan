// Package ocrstore keeps immutable OCR text and completed checks.
package ocrstore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

const maxText = 50_000_000

// Store indexes text files in the workshop database.
type Store struct {
	root string
	q    *store.Queries
}

// Open constructs a store for the given directory and database.
func Open(root string, db store.DBTX) *Store { return &Store{root: root, q: store.New(db)} }

func validHash(h string) bool {
	if len(h) != 64 || strings.ToLower(h) != h {
		return false
	}
	_, err := hex.DecodeString(h)
	return err == nil
}

// ReadText reads and verifies the bytes named by an entry.
func (s *Store) ReadText(entry store.OcrText) ([]byte, error) {
	if !validHash(entry.TextSha256) {
		return nil, fmt.Errorf("invalid text hash")
	}
	path := filepath.Join(s.root, "text", entry.TextSha256+".txt")
	info, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("stat OCR text: %w", err)
	}
	if !info.Mode().IsRegular() || info.Size() > maxText {
		return nil, fmt.Errorf("invalid OCR text file")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read OCR text: %w", err)
	}
	if fmt.Sprintf("%x", sha256.Sum256(data)) != entry.TextSha256 || !utf8.Valid(data) {
		return nil, fmt.Errorf("OCR text verification failed")
	}
	return data, nil
}

// Put stores text and returns true if the PDF and engine already had an entry.
func (s *Store) Put(ctx context.Context, pdfHash, engine, origin string, pages pgtype.Int4, data []byte) (bool, error) {
	if !validHash(pdfHash) || engine == "" || len(data) == 0 || len(data) > maxText || !utf8.Valid(data) {
		return false, fmt.Errorf("invalid OCR text or identity")
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	dir := filepath.Join(s.root, "text")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return false, fmt.Errorf("create text directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".ocr-")
	if err != nil {
		return false, fmt.Errorf("create temporary OCR text: %w", err)
	}
	defer func() { _ = os.Remove(tmp.Name()) }()
	if _, err = tmp.Write(data); err == nil {
		err = tmp.Sync()
	}
	closeErr := tmp.Close()
	if err != nil {
		return false, fmt.Errorf("write OCR text: %w", err)
	}
	if closeErr != nil {
		return false, fmt.Errorf("close OCR text: %w", closeErr)
	}
	if err = os.Link(tmp.Name(), filepath.Join(dir, hash+".txt")); err != nil && !errors.Is(err, os.ErrExist) {
		return false, fmt.Errorf("publish OCR text: %w", err)
	}
	if _, err = s.ReadText(store.OcrText{TextSha256: hash}); err != nil {
		return false, err
	}
	_, err = s.q.InsertOCRText(ctx, store.InsertOCRTextParams{PdfSha256: pdfHash, EngineID: engine, TextSha256: hash, Bytes: int64(len(data)), Origin: origin, Pages: pages})
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert OCR text: %w", err)
	}
	return false, nil
}

// Entry pairs the first accepted text with its completed check, if present.
type Entry struct {
	Text  store.OcrText
	Check *store.OcrCheck
}

// Lookup selects the first available engine for each PDF.
func (s *Store) Lookup(ctx context.Context, hashes, engines []string, checker string) (map[string]Entry, error) {
	rows, err := s.q.ListOCRTextByPDF(ctx, hashes)
	if err != nil {
		return nil, fmt.Errorf("list OCR texts: %w", err)
	}
	byKey := make(map[string]store.OcrText, len(rows))
	for _, row := range rows {
		byKey[row.PdfSha256+"\x00"+row.EngineID] = row
	}
	result := make(map[string]Entry)
	for _, hash := range hashes {
		for _, engine := range engines {
			row, ok := byKey[hash+"\x00"+engine]
			if !ok {
				continue
			}
			entry := Entry{Text: row}
			if checker != "" {
				check, checkErr := s.q.GetOCRCheck(ctx, store.GetOCRCheckParams{OcrTextID: row.ID, CheckerID: checker})
				if checkErr != nil && !errors.Is(checkErr, pgx.ErrNoRows) {
					return nil, fmt.Errorf("get OCR check: %w", checkErr)
				}
				if checkErr == nil {
					entry.Check = &check
				}
			}
			result[hash] = entry
			break
		}
	}
	return result, nil
}

// PutCheck stores a completed check as canonical JSON and reports a conflict.
func (s *Store) PutCheck(ctx context.Context, textID int64, checker string, result json.RawMessage, flagged int32, completed bool) (bool, error) {
	if !completed || checker == "" || flagged < 0 {
		return false, fmt.Errorf("invalid or incomplete OCR check")
	}
	var object map[string]any
	decoder := json.NewDecoder(bytes.NewReader(result))
	decoder.UseNumber()
	if err := decoder.Decode(&object); err != nil || object == nil {
		return false, fmt.Errorf("invalid OCR check result")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return false, fmt.Errorf("invalid OCR check result")
	}
	canonical, err := json.Marshal(object)
	if err != nil {
		return false, fmt.Errorf("encode OCR check: %w", err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(canonical))
	_, err = s.q.InsertOCRCheck(ctx, store.InsertOCRCheckParams{OcrTextID: textID, CheckerID: checker, ResultSha256: hash, Result: canonical, Flagged: flagged})
	if errors.Is(err, pgx.ErrNoRows) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("insert OCR check: %w", err)
	}
	return false, nil
}
