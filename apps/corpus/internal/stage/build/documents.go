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
	"danny.vn/trachuan/internal/retrieval"
)

type loadedDocument struct {
	id               int64
	key, number      string
	nodes            map[string]int64
	anchors          map[string]string
	chunk, embedding *InventoryItem
}
type fetchDetail struct {
	Data struct {
		ID, DocNum, Title, AgencyName, IssueDate, EffFrom, EffTo string
		DocType, EffStatus                                       struct{ Code, Name string }
		IsConsolidatedDocument                                   bool
	} `json:"data"`
}
type fetchedFile struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
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
	if len(l.inv.ExpectedKeys) > 10000 {
		return fmt.Errorf("document limit exceeded")
	}
	for _, key := range l.inv.ExpectedKeys {
		if l.excluded(key) {
			continue
		}
		item, ok := l.items["fetch"][key]
		if !ok || item.State != "done" {
			continue
		}
		dir, err := artifactDir(l.artifacts, item)
		if err != nil {
			return fmt.Errorf("fetch artifact: %w", err)
		}
		var detail fetchDetail
		if err = readArtifactJSON(dir, "detail.json", &detail); err != nil {
			return err
		}
		d := detail.Data
		if "vn:vbpl:"+d.ID != key {
			return fmt.Errorf("fetch identity mismatch")
		}
		var files []fetchedFile
		if err = readArtifactJSON(dir, "files.json", &files); err != nil {
			return err
		}
		sort.Slice(files, func(i, j int) bool {
			if files[i].Name != files[j].Name {
				return files[i].Name < files[j].Name
			}
			return files[i].SHA256 < files[j].SHA256
		})
		var nodes []treeNode
		var extractHash *string
		ext, exists := l.items["extract"][key]
		if exists && ext.State == "done" {
			if err = checkInputHash(ext, *item.ArtifactSHA256); err != nil {
				return err
			}
			p, e := artifactDir(l.artifacts, ext)
			if e != nil {
				return fmt.Errorf("extract artifact: %w", e)
			}
			var raw []chunk.Node
			if e = readArtifactJSON(p, "provisions.json", &raw); e != nil {
				return e
			}
			nodes, e = preorder(raw)
			if e != nil {
				l.addGap("build", key, "invalid_ancestry", "")
				nodes = nil
			}
			extractHash = ext.ArtifactSHA256
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
		class, current, known := Status(d.EffStatus.Code, l.codes)
		derived := d.IsConsolidatedDocument && strings.TrimSpace(d.EffStatus.Code) == "" && strings.TrimSpace(d.EffStatus.Name) == ""
		basis := "source_status"
		if derived {
			basis = "derived_from_consolidated"
		}
		if !known && !derived {
			l.addGap("build", key, "unknown_status", d.EffStatus.Code)
		}
		id := l.counts.Documents + 1
		_, err = l.tx.Exec(ctx, `INSERT INTO document(id,country,key,number,number_norm,title,doc_type,issuer,issue_date,eff_from,eff_to,raw_issue_date,raw_eff_from,raw_eff_to,status_code,status_name,status_class,status_basis,is_current,is_consolidated,content_state,fetch_sha256,extract_sha256,source_url) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,NULL)`, id, l.inv.Country, key, d.DocNum, retrieval.NormalizeNumber(d.DocNum), d.Title, d.DocType.Name, d.AgencyName, issue, from, to, d.IssueDate, d.EffFrom, d.EffTo, d.EffStatus.Code, d.EffStatus.Name, class, basis, current, d.IsConsolidatedDocument, state, *item.ArtifactSHA256, extractHash)
		if err != nil {
			return fmt.Errorf("insert document: %w", err)
		}
		l.counts.Documents++
		doc := &loadedDocument{id: id, key: key, number: d.DocNum, nodes: map[string]int64{}, anchors: map[string]string{}}
		if hasChunk && ci.State == "done" {
			doc.chunk = &ci
		}
		if hasEmbed && ei.State == "done" {
			doc.embedding = &ei
		}
		l.docs[key] = doc
		l.addGap("build", key, "source_provenance_missing", "url,role,fetched_at")
		for _, file := range files {
			e := checkSourceFile(filepath.Join(dir, "files", file.SHA256), file.Size, file.SHA256)
			if e != nil {
				return e
			}
			_, e = l.tx.Exec(ctx, `INSERT INTO source_file(id,document_id,source,name,hash,size,role) VALUES($1,$2,$3,$4,$5,$6,'unknown')`, l.counts.SourceFiles+1, id, "vbpl", file.Name, file.SHA256, file.Size)
			if e != nil {
				return fmt.Errorf("insert source file: %w", e)
			}
			l.counts.SourceFiles++
		}
		localIDs := make([]int64, 0, len(nodes))
		for i, n := range nodes {
			var parent any
			if n.Parent >= 0 {
				parent = localIDs[n.Parent]
			}
			pid := l.counts.Provisions + 1
			_, e := l.tx.Exec(ctx, `INSERT INTO provision(id,document_id,parent_id,node_id,kind,label,heading,ordinal,preorder,path,text,anchor) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::ltree,$11,$12)`, pid, id, parent, n.Node.NodeID, n.Node.Kind, n.Node.Label, n.Node.Heading, n.Node.Ordinal, i+1, n.Path, n.Node.Text, n.Node.Path)
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
