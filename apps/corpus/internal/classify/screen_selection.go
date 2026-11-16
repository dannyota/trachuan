package classify

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

type DocumentSelection struct {
	LabelsHash   string   `json:"labels_sha256"`
	DocumentKeys []string `json:"document_keys"`
}

func (s DocumentSelection) validate() error {
	hash, err := hex.DecodeString(s.LabelsHash)
	if err != nil || len(hash) != 32 || len(s.DocumentKeys) == 0 {
		return fmt.Errorf("document selection requires labels SHA-256 and document keys")
	}
	for i, key := range s.DocumentKeys {
		if key == "" || (i > 0 && key <= s.DocumentKeys[i-1]) {
			return fmt.Errorf("selected document keys must be distinct and sorted")
		}
	}
	return nil
}

func SelectLabelDocuments(labels []taxonomy.LabelArticle, primary []Document) ([]Document, error) {
	byKey := map[string]Document{}
	for _, d := range primary {
		byKey[d.Key] = d
	}
	selected := map[string]bool{}
	var documents []Document
	for _, label := range labels {
		if selected[label.Key] {
			continue
		}
		d, ok := byKey[label.Key]
		if !ok {
			return nil, fmt.Errorf("selected document %s is absent or not primary in the release", label.Key)
		}
		selected[label.Key] = true
		documents = append(documents, d)
	}
	sort.Slice(documents, func(i, j int) bool { return documents[i].Key < documents[j].Key })
	return documents, nil
}

func documentSelectionHash(o Options) string {
	b, _ := json.Marshal(struct {
		RunID                        int64
		Country, Release, RecipeHash string
		Selection                    *DocumentSelection
	}{o.RunID, o.Country, o.Release, o.Recipe.Hash(), o.Selection})
	return digest(b)
}

func screenDocuments(o Options, docs []Document) ([]Document, error) {
	if o.Selection == nil {
		return draw(docs, o.Documents, o.Seed)
	}
	byKey := map[string]Document{}
	for _, d := range docs {
		byKey[d.Key] = d
	}
	selected := make([]Document, 0, len(o.Selection.DocumentKeys))
	for _, key := range o.Selection.DocumentKeys {
		d, ok := byKey[key]
		if !ok {
			return nil, fmt.Errorf("selected document %s has no release evidence", key)
		}
		selected = append(selected, d)
	}
	return selected, nil
}
