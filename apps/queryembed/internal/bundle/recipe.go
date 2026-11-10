package bundle

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"danny.vn/trachuan/internal/embedding"
)

// QueryModelPath is the INT8 model file used for queries.
const QueryModelPath = "onnx/model_int8.onnx"

// Versions contains the bare versions of the query service dependencies.
type Versions struct{ Go, Tokenizers, ONNXRuntime string }

// TokenizerFiles selects tokenizer entries in manifest order.
func TokenizerFiles(m Manifest) []File {
	var files []File
	for _, f := range m.Files {
		if strings.Contains(f.Path, "token") || strings.HasSuffix(f.Path, "sentencepiece.bpe.model") {
			files = append(files, f)
		}
	}
	return files
}

func manifestHash(files []File) (string, error) {
	// Python sorts object keys when hashing each manifest entry.
	type entry struct {
		Path   string `json:"path"`
		SHA256 string `json:"sha256"`
		Size   int64  `json:"size"`
	}
	items := make([]entry, 0, len(files))
	for _, f := range files {
		items = append(items, entry{f.Path, f.SHA256, f.Size})
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(items); err != nil {
		return "", fmt.Errorf("encode manifest entries: %w", err)
	}
	data := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	data = unescapeSeparators(data)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// Go escapes these two characters even when HTML escaping is disabled.
func unescapeSeparators(data []byte) []byte {
	var out []byte
	for i := 0; i < len(data); {
		if data[i] == '\\' && i+1 < len(data) {
			if i+6 <= len(data) && (string(data[i:i+6]) == `\u2028` || string(data[i:i+6]) == `\u2029`) {
				if data[i+5] == '8' {
					out = append(out, "\u2028"...)
				} else {
					out = append(out, "\u2029"...)
				}
				i += 6
				continue
			}
			out = append(out, data[i], data[i+1])
			i += 2
			continue
		}
		out = append(out, data[i])
		i++
	}
	return out
}

// QueryRecipe builds the query runtime recipe from the bundle manifest.
func QueryRecipe(m Manifest, v Versions) (embedding.Recipe, error) {
	if err := m.validate(); err != nil {
		return embedding.Recipe{}, fmt.Errorf("validate manifest: %w", err)
	}
	tokenizer := TokenizerFiles(m)
	var weights []File
	for _, f := range m.Files {
		if f.Path == QueryModelPath {
			weights = append(weights, f)
		}
	}
	if len(tokenizer) == 0 || len(weights) != 1 {
		return embedding.Recipe{}, fmt.Errorf("bundle lacks tokenizer or query weights")
	}
	th, err := manifestHash(tokenizer)
	if err != nil {
		return embedding.Recipe{}, err
	}
	wh, err := manifestHash(weights)
	if err != nil {
		return embedding.Recipe{}, err
	}
	return embedding.Recipe{Model: m.Model, Revision: m.Revision, ResolvedRevision: m.ResolvedRevision, TokenizerRevision: m.ResolvedRevision, TokenizerSHA256: th, WeightsSHA256: wh, Dimensions: 1024, Pooling: "cls", Normalization: "l2-float32", Input: "raw", Runtime: "query", Backend: "onnxruntime-cpu", Precision: "int8", TokenCap: 256, SpecialTokens: true, Truncation: false, Language: "go " + v.Go, TokenizerLibrary: "daulet/tokenizers " + v.Tokenizers, RuntimeLibrary: "onnxruntime " + v.ONNXRuntime}, nil
}
