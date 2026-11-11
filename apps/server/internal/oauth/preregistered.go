package oauth

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// LoadPreregistered reads an optional JSON array of client_id, client_name and
// redirect_uris objects. Configuration errors fail startup without file values.
func LoadPreregistered(path string) ([]Client, error) {
	if path == "" {
		return nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open preregistered clients: %w", err)
	}
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, 4*1024*1024+1))
	if err != nil {
		return nil, fmt.Errorf("read preregistered clients: %w", err)
	}
	if len(raw) > 4*1024*1024 {
		return nil, ErrInvalidClient
	}
	var entries []json.RawMessage
	if json.Unmarshal(raw, &entries) != nil || entries == nil || len(entries) > 10000 {
		return nil, ErrInvalidClient
	}
	clients := make([]Client, 0, len(entries))
	seen := map[string]bool{}
	for _, entry := range entries {
		fields, err := metadataObject(entry)
		if err != nil {
			return nil, ErrInvalidClient
		}
		var id string
		if !metadataString(fields, "client_id", &id) || id == "" || len(id) > 2048 || strings.HasPrefix(id, "https://") || strings.ContainsAny(id, " \t\r\n") || seen[id] {
			return nil, ErrInvalidClient
		}
		c, code := clientMetadata(fields)
		if code != "" {
			return nil, ErrInvalidClient
		}
		c.ID = id
		c.Kind = "preregistered"
		seen[id] = true
		clients = append(clients, c)
	}
	return clients, nil
}
