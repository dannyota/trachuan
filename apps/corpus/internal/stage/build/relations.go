package build

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate"
	"danny.vn/trachuan/internal/retrieval"
)

type relationRow struct {
	Ordinal      int     `json:"ordinal"`
	Kind         string  `json:"kind"`
	RawCode      string  `json:"raw_code"`
	RawLabel     string  `json:"raw_label"`
	Method       string  `json:"method"`
	Evidence     string  `json:"evidence"`
	TargetID     string  `json:"target_vbpl_id"`
	TargetNumber string  `json:"target_number"`
	SourceNode   *string `json:"source_node_id"`
	TargetNode   *string `json:"target_node_id"`
}
type pendingRelation struct {
	key, hash string
	row       relationRow
}

func checkRelationInput(item, fetchItem, extractItem InventoryItem, found bool) error {
	if fetchItem.ArtifactSHA256 == nil {
		return fmt.Errorf("missing relation fetch artifact")
	}
	if !found {
		return fmt.Errorf("missing relation extract item")
	}
	extract := "-"
	if extractItem.State == "done" && extractItem.ArtifactSHA256 != nil {
		extract = *extractItem.ArtifactSHA256
	} else if (extractItem.State != "gap" && extractItem.State != "failed") || extractItem.ArtifactSHA256 != nil {
		return fmt.Errorf("unfinished or invalid relation extract item")
	}
	// A worker refuses runs with a different frozen relate recipe, so build only sees pair hashes.
	return checkInputHash(item, relate.PairInputHash(*fetchItem.ArtifactSHA256, extract))
}

func docID(d *loadedDocument) any {
	if d == nil {
		return nil
	}
	return d.id
}
func nodeID(d *loadedDocument, n *string) any {
	if d == nil || n == nil {
		return nil
	}
	id, ok := d.nodes[*n]
	if !ok {
		return nil
	}
	return id
}
func (l *loader) loadRelations(ctx context.Context) error {
	var entries []pendingRelation
	for key, item := range l.items["relate"] {
		if item.State != "done" || (l.inv.Version == 2 && (!l.present[key] || strings.HasPrefix(key, "vn:chinhphu:"))) {
			continue
		}
		fetch, ok := l.items["fetch"][key]
		if !ok || fetch.ArtifactSHA256 == nil {
			return fmt.Errorf("missing relation source")
		}
		extract, found := l.items["extract"][key]
		if err := checkRelationInput(item, fetch, extract, found); err != nil {
			return err
		}
		dir, err := artifactDir(l.artifacts, item)
		if err != nil {
			return fmt.Errorf("relation artifact: %w", err)
		}
		var rows []relationRow
		if err = readArtifactJSON(dir, "relations.json", &rows); err != nil {
			return err
		}
		for _, row := range rows {
			entries = append(entries, pendingRelation{key, *item.ArtifactSHA256, row})
		}
	}
	// Documents can have byte-identical relation artifacts, so the source key
	// leads the order and the unique key.
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].key != entries[j].key {
			return entries[i].key < entries[j].key
		}
		if entries[i].hash != entries[j].hash {
			return entries[i].hash < entries[j].hash
		}
		return entries[i].row.Ordinal < entries[j].row.Ordinal
	})
	numbers := l.documentNumbers()
	for _, entry := range entries {
		r := entry.row
		source := l.docs[entry.key]
		var target *loadedDocument
		var targetKey any
		if r.TargetID != "" {
			key := "vn:vbpl:" + r.TargetID
			targetKey = key
			target = l.docs[key]
		} else if key := resolveNumber(numbers, r.TargetNumber); key != "" {
			targetKey = key
			target = l.docs[key]
		}
		srcNumber := ""
		if source != nil {
			srcNumber = source.number
		} else if fetchItem, ok := l.items["fetch"][entry.key]; ok && fetchItem.State == "done" {
			dir, err := artifactDir(l.artifacts, fetchItem)
			if err != nil {
				return fmt.Errorf("relation source artifact: %w", err)
			}
			detail, err := fetch.ReadDocument(dir, entry.key)
			if err != nil {
				return fmt.Errorf("decode relation source: %w", err)
			}
			srcNumber = detail.Number
		} else {
			l.addGap("build", entry.key, "source_provenance_missing", "relation source number")
		}
		srcNode, dstNode := nodeID(source, r.SourceNode), nodeID(target, r.TargetNode)
		if (r.SourceNode != nil && srcNode == nil) || (r.TargetNode != nil && dstNode == nil) {
			l.addGap("build", entry.key, "unresolved_provision_relation", fmt.Sprintf("%s:%d", entry.hash, r.Ordinal))
		}
		_, err := l.tx.Exec(ctx, `INSERT INTO relation(id,source_document_id,target_document_id,source_key,target_key,source_number,target_number,kind,raw_code,raw_label,source_provision_id,target_provision_id,source_node_id,target_node_id,evidence,method,artifact_sha256,ordinal) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18)`, l.counts.Relations+1, docID(source), docID(target), entry.key, targetKey, srcNumber, r.TargetNumber, r.Kind, r.RawCode, r.RawLabel, srcNode, dstNode, r.SourceNode, r.TargetNode, r.Evidence, r.Method, entry.hash, r.Ordinal)
		if err != nil {
			return fmt.Errorf("insert relation: %w", err)
		}
		l.counts.Relations++
	}
	return nil
}

func resolveNumber(numbers map[string][]string, number string) string {
	matches := numbers[retrieval.NormalizeNumber(number)]
	if len(matches) != 1 {
		return ""
	}
	return matches[0]
}

func (l *loader) documentNumbers() map[string][]string {
	numbers := map[string][]string{}
	for key, doc := range l.docs {
		if number := retrieval.NormalizeNumber(doc.number); number != "" {
			numbers[number] = append(numbers[number], key)
		}
	}
	return numbers
}
