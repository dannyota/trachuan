package contracttest

import (
	"fmt"
	"mime"
	"net/http/httptest"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"
)

type responseKey struct{ method, path, status, media string }

// Coverage tracks every response documented for the selected paths.
type Coverage struct {
	contract *Contract
	expected map[responseKey]bool
}

// NewCoverage creates a coverage tracker for all operations at paths.
func NewCoverage(c *Contract, paths []string) *Coverage {
	result := &Coverage{contract: c, expected: make(map[responseKey]bool)}
	for _, path := range paths {
		for method, op := range c.Paths[path] {
			for status, response := range op.Responses {
				if len(response.Content) == 0 {
					result.expected[responseKey{method, path, status, ""}] = false
				}
				for media := range response.Content {
					result.expected[responseKey{method, path, status, media}] = false
				}
			}
		}
	}
	return result
}

// NewCoverageForTag tracks responses of operations with the given owner tag.
func NewCoverageForTag(c *Contract, tag string) *Coverage {
	result := &Coverage{contract: c, expected: make(map[responseKey]bool)}
	for path, methods := range c.Paths {
		for method, op := range methods {
			if !slices.Contains(op.Tags, tag) {
				continue
			}
			for status, response := range op.Responses {
				if len(response.Content) == 0 {
					result.expected[responseKey{method, path, status, ""}] = false
				}
				for media := range response.Content {
					result.expected[responseKey{method, path, status, media}] = false
				}
			}
		}
	}
	return result
}

// Check validates an observed response and records its coverage.
func (c *Coverage) Check(t *testing.T, method, path string, w *httptest.ResponseRecorder) {
	t.Helper()
	if err := c.observe(method, path, w); err != nil {
		t.Fatal(err)
	}
}

func (c *Coverage) observe(method, path string, w *httptest.ResponseRecorder) error {
	op, ok := c.contract.Paths[path][strings.ToLower(method)]
	if !ok {
		return fmt.Errorf("undocumented operation %s %s", method, path)
	}
	status := strconv.Itoa(w.Code)
	response, ok := op.Responses[status]
	if !ok {
		return fmt.Errorf("undocumented response %s %s %s", method, path, status)
	}
	mediaType := ""
	if len(w.Body.Bytes()) > 0 {
		var err error
		mediaType, _, err = mime.ParseMediaType(w.Header().Get("Content-Type"))
		if err != nil {
			return fmt.Errorf("parse response media type: %w", err)
		}
	}
	key := responseKey{strings.ToLower(method), path, status, mediaType}
	if _, ok := c.expected[key]; !ok {
		return fmt.Errorf("undocumented media %s for %s %s %s", mediaType, method, path, status)
	}
	if mediaType != "" {
		schema := response.Content[mediaType].Schema
		var value any = w.Body.String()
		if mediaType == "application/json" {
			var err error
			value, err = decodeJSON(w.Body.Bytes())
			if err != nil {
				return err
			}
		}
		if err := c.contract.validate(schema, value); err != nil {
			return fmt.Errorf("%s %s %s: %w", method, path, status, err)
		}
	}
	c.expected[key] = true
	return nil
}

// Missing lists the documented responses that no handler case exercised.
func (c *Coverage) Missing() []string {
	var missing []string
	for key, seen := range c.expected {
		if !seen {
			missing = append(missing, fmt.Sprintf("%s %s %s %s", key.method, key.path, key.status, key.media))
		}
	}
	sort.Strings(missing)
	return missing
}
