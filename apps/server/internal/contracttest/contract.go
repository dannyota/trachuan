// Package contracttest checks handlers against the web OpenAPI contract.
package contracttest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"gopkg.in/yaml.v3"
)

type media struct {
	Schema map[string]any `yaml:"schema"`
}
type response struct {
	Content map[string]media `yaml:"content"`
}
type operation struct {
	Responses   map[string]response `yaml:"responses"`
	RequestBody struct {
		Content map[string]media `yaml:"content"`
	} `yaml:"requestBody"`
}

// Contract is a parsed OpenAPI document and its resolved schemas.
type Contract struct {
	Paths      map[string]map[string]operation `yaml:"paths"`
	Components struct {
		Schemas map[string]map[string]any `yaml:"schemas"`
	} `yaml:"components"`
}

// Load reads the contract from the repository containing the calling package.
func Load() (*Contract, error) {
	dir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("get working directory: %w", err)
	}
	for {
		filename := filepath.Join(dir, "packages/schema/openapi.yaml")
		raw, readErr := os.ReadFile(filename)
		if readErr == nil {
			var c Contract
			if err := yaml.Unmarshal(raw, &c); err != nil {
				return nil, fmt.Errorf("parse contract: %w", err)
			}
			return &c, nil
		}
		if !os.IsNotExist(readErr) {
			return nil, fmt.Errorf("read contract: %w", readErr)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, fmt.Errorf("find contract: %w", readErr)
		}
		dir = parent
	}
}

func (c *Contract) validate(schema map[string]any, value any) error {
	root := map[string]any{"$defs": c.Components.Schemas, "allOf": []any{schema}}
	raw, err := json.Marshal(root)
	if err != nil {
		return fmt.Errorf("encode schema: %w", err)
	}
	raw = []byte(strings.ReplaceAll(string(raw), "#/components/schemas/", "#/$defs/"))
	var doc jsonschema.Schema
	if err = json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("decode schema: %w", err)
	}
	resolved, err := doc.Resolve(nil)
	if err != nil {
		return fmt.Errorf("resolve schema: %w", err)
	}
	if err = resolved.Validate(value); err != nil {
		return fmt.Errorf("validate schema: %w", err)
	}
	return nil
}

func decodeJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("decode JSON: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("decode JSON: trailing value")
	}
	return normalizeNumbers(value), nil
}

// ValidateRequest validates a JSON request body against its documented schema.
func (c *Contract) ValidateRequest(method, path string, body []byte) error {
	op, ok := c.Paths[path][strings.ToLower(method)]
	if !ok {
		return fmt.Errorf("undocumented operation %s %s", method, path)
	}
	m, ok := op.RequestBody.Content["application/json"]
	if !ok {
		return fmt.Errorf("undocumented JSON request %s %s", method, path)
	}
	value, err := decodeJSON(body)
	if err != nil {
		return err
	}
	return c.validate(m.Schema, value)
}

// Preserve integer values and avoid rounding a fractional JSON number to an integer.
func normalizeNumbers(value any) any {
	switch v := value.(type) {
	case json.Number:
		n, ok := new(big.Rat).SetString(v.String())
		if !ok {
			return v
		}
		if n.IsInt() {
			if n.Num().IsInt64() {
				return n.Num().Int64()
			}
			if n.Num().IsUint64() {
				return n.Num().Uint64()
			}
		}
		f, _ := n.Float64()
		if !n.IsInt() && !math.IsInf(f, 0) && math.Trunc(f) == f {
			rounded := new(big.Rat).SetFloat64(f)
			if n.Cmp(rounded) > 0 {
				f = math.Nextafter(f, math.Inf(1))
			} else {
				f = math.Nextafter(f, math.Inf(-1))
			}
		}
		return f
	case []any:
		for i, item := range v {
			v[i] = normalizeNumbers(item)
		}
	case map[string]any:
		for key, item := range v {
			v[key] = normalizeNumbers(item)
		}
	}
	return value
}
