package build

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/fetch"
	"danny.vn/trachuan/internal/retrieval"
)

type loadedDocument struct {
	id               int64
	key, number      string
	nodes            map[string]int64
	anchors          map[string]string
	chunk, embedding *InventoryItem
}

func readArtifactJSON(dir, name string, dst any) error {
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		return fmt.Errorf("read artifact JSON: %w", err)
	}
	if err = json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("decode artifact JSON: %w", err)
	}
	return nil
}
func (l *loader) addGap(stage, key, kind, detail string) {
	l.gaps = append(l.gaps, InventoryGap{Stage: stage, ItemKey: key, Kind: kind, Detail: detail})
}
func (l *loader) date(key, field, raw string) *time.Time {
	d, err := ParseDate(raw)
	if err != nil {
		l.addGap("build", key, "invalid_date", field)
	}
	return d
}
func (l *loader) excluded(key string) bool {
	for _, gap := range l.gaps {
		if gap.ItemKey == key && (gap.Kind == "withdrawn" || gap.Kind == "scope_removed") {
			return true
		}
	}
	return false
}
func (l *loader) loadDocuments(ctx context.Context) error {
	for _, key := range l.inv.ExpectedKeys {
		if !l.present[key] {
			continue
		}
		if l.counts.Documents >= 10000 {
			return fmt.Errorf("document limit exceeded")
		}
		item, ok := l.items["fetch"][key]
		if !ok || item.State != "done" {
			continue
		}
		dir, err := artifactDir(l.artifacts, item)
		if err != nil {
			return fmt.Errorf("fetch artifact: %w", err)
		}
		d, err := fetch.ReadDocument(dir, key)
		if err != nil {
			return fmt.Errorf("decode fetch artifact: %w", err)
		}
		files := d.Files
		sort.Slice(files, func(i, j int) bool {
			if files[i].Name != files[j].Name {
				return files[i].Name < files[j].Name
			}
			return files[i].SHA256 < files[j].SHA256
		})

		var nodes []treeNode
		var extractHash *string
		textSource := "none"
		var locators map[string]string
		ext, exists := l.items["extract"][key]
		if exists && ext.State == "done" {
			p, e := artifactDir(l.artifacts, ext)
			if e != nil {
				return fmt.Errorf("extract artifact: %w", e)
			}
			if ext.InputHash == nil {
				return fmt.Errorf("missing extract input hash")
			}
			if e = fetch.CheckExtractLineage(p, *item.ArtifactSHA256, *ext.InputHash); e != nil {
				return fmt.Errorf("extract lineage: %w", e)
			}
			textSource, locators, e = readTextMetadata(p)
			if e != nil {
				return e
			}
			var raw []chunk.Node
			if e = readArtifactJSON(p, "provisions.json", &raw); e != nil {
				return e
			}
			if textSource == "none" && len(raw) > 0 {
				return fmt.Errorf("text source none has provisions for %s", key)
			}
			nodes, e = preorder(raw)
			if e != nil {
				l.addGap("build", key, "invalid_ancestry", "")
				nodes = nil
			}
			extractHash = ext.ArtifactSHA256
		}
		if strings.HasPrefix(key, "vn:chinhphu:") && len(nodes) == 0 {
			hasGap := false
			for _, gap := range l.gaps {
				if gap.ItemKey == key && gap.Kind == "no_tree" {
					hasGap = true
					break
				}
			}
			if !hasGap {
				l.addGap("build", key, "no_tree", "")
			}
		}
		if l.counts.Provisions+int64(len(nodes)) > 2000000 {
			return fmt.Errorf("provision limit exceeded")
		}
		ci, hasChunk := l.items["chunk"][key]
		ei, hasEmbed := l.items["embed"][key]
		state := "no_tree"
		if len(nodes) > 0 {
			state = "no_chunks"
			if hasChunk && ci.State == "done" {
				state = "embedding_gap"
				if hasEmbed && ei.State == "done" {
					state = "indexed"
				}
			}
		}
		issue, from, to := l.date(key, "issue_date", d.IssueDate), l.date(key, "eff_from", d.EffFrom), l.date(key, "eff_to", d.EffTo)
		if from != nil && to != nil && to.Before(*from) {
			from = nil
			to = nil
			l.addGap("build", key, "invalid_validity_interval", "")
		}
		if from == nil {
			l.addGap("build", key, "missing_effective_date", "")
		}
		class, current, known := Status(d.StatusCode, l.codes)
		derived := d.IsConsolidated && strings.TrimSpace(d.StatusCode) == "" && strings.TrimSpace(d.StatusName) == ""
		basis := "source_status"
		if derived {
			basis = "derived_from_consolidated"
		}
		if !known && !derived {
			l.addGap("build", key, "unknown_status", d.StatusCode)
		}
		id := l.counts.Documents + 1
		docClass, docSource := "primary", "vbpl"
		if l.inv.Version == 2 {
			member := l.members[key]
			docClass, docSource = member.Class, member.Source
		}
		since, rel, err := l.pinnedMetadata(key, *item.ArtifactSHA256)
		if err != nil {
			return err
		}
		_, err = l.tx.Exec(ctx, `INSERT INTO document(id,country,key,number,number_norm,title,doc_type,issuer,class,source,validity_conflict,issue_date,eff_from,eff_to,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,extract_sha256,source_url,text_source,text_complete,source_withdrawn_since,source_withdrawn_rel) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$24,$25,false,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$26,$27,$28,$29,$30)`, id, l.inv.Country, key, d.Number, retrieval.NormalizeNumber(d.Number), d.Title, d.DocType, d.Issuer, issue, from, to, d.IssueDate, d.EffFrom, d.EffTo, d.StatusCode, d.StatusName, class, basis, current, d.IsConsolidated, state, *item.ArtifactSHA256, extractHash, docClass, docSource, d.SourceURL, textSource, textComplete(len(nodes), key, l.gaps), since, rel)
		if err != nil {
			return fmt.Errorf("insert document: %w", err)
		}
		l.counts.Documents++
		doc := &loadedDocument{id: id, key: key, number: d.Number, nodes: map[string]int64{}, anchors: map[string]string{}}
		if hasChunk && ci.State == "done" {
			doc.chunk = &ci
		}
		if hasEmbed && ei.State == "done" {
			doc.embedding = &ei
		}
		l.docs[key] = doc
		if err = l.loadSourceFiles(ctx, key, docSource, dir, id, files); err != nil {
			return err
		}
		localIDs := make([]int64, 0, len(nodes))
		for i, n := range nodes {
			var parent any
			if n.Parent >= 0 {
				parent = localIDs[n.Parent]
			}
			var locator any
			if textSource != "source_tree" {
				value, ok := locators[n.Node.NodeID]
				if !ok {
					return fmt.Errorf("missing locator for provision %s", n.Node.NodeID)
				}
				locator = value
			}
			pid := l.counts.Provisions + 1
			_, e := l.tx.Exec(ctx, `INSERT INTO provision(id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor,locator) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::ltree,$11,$12,$13)`, pid, id, parent, n.Node.NodeID, n.Node.Kind, n.Node.Label, n.Node.Heading, n.Node.Ordinal, i+1, n.Path, n.Node.Text, n.Node.Path, locator)
			if e != nil {
				return fmt.Errorf("insert provision: %w", e)
			}
			localIDs = append(localIDs, pid)
			doc.nodes[n.Node.NodeID] = pid
			doc.anchors[n.Node.NodeID] = n.Node.Path
			l.counts.Provisions++
		}
	}
	return nil
}
