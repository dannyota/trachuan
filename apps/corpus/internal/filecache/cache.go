// Package filecache indexes previously collected VBPL attachments.
package filecache

import (
	"encoding/csv"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type key struct {
	id, name string
	size     int64
}
type entry struct {
	path, hash string
}

// Cache holds the attachment index in memory.
type Cache struct{ entries map[key][]entry }

// Load reads the cache index from dir.
func Load(dir string) (*Cache, error) {
	f, err := os.Open(filepath.Join(dir, "index.tsv"))
	if err != nil {
		return nil, fmt.Errorf("open cache index: %w", err)
	}
	defer func() { _ = f.Close() }()
	reader := csv.NewReader(f)
	reader.Comma = '\t'
	reader.FieldsPerRecord = 6
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read cache header: %w", err)
	}
	if strings.Join(header, "\t") != "vbpl_id\tname\tsize\tsha256\tformat\tcollected_at" {
		return nil, fmt.Errorf("invalid cache header")
	}
	c := &Cache{entries: make(map[key][]entry)}
	for {
		row, readErr := reader.Read()
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return nil, fmt.Errorf("read cache index: %w", readErr)
		}
		size, sizeErr := strconv.ParseInt(row[2], 10, 64)
		digest, hashErr := hex.DecodeString(row[3])
		if sizeErr != nil || size < 0 || hashErr != nil || len(digest) != 32 || row[3] != strings.ToLower(row[3]) || row[4] == "" || strings.ContainsAny(row[4], "/\\") || row[4] == "." || row[4] == ".." {
			return nil, fmt.Errorf("invalid cache index entry")
		}
		k := key{row[0], row[1], size}
		c.entries[k] = append(c.entries[k], entry{filepath.Join(dir, "files", row[3]+"."+row[4]), row[3]})
	}
	return c, nil
}

// Lookup returns a size-checked entry for a document and file name.
func (c *Cache) Lookup(vbplID, name string, size int64) (path, sha256 string, ok bool) {
	if c == nil {
		return "", "", false
	}
	for _, e := range c.entries[key{vbplID, name, size}] {
		return e.path, e.hash, true
	}
	return "", "", false
}
