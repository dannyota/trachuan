//go:build integration

package retrieval

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestSchemaCompatibility_Version6To8Serve(t *testing.T) {
	for _, version := range []int{6, 7, 8} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			db, _, embedder := fixture(t)
			ctx := t.Context()
			textSource := "source_tree"
			if version == 6 {
				readExec(t, db, `DROP TABLE document_summary;
ALTER TABLE release DROP CONSTRAINT release_schema_version_check;
UPDATE release SET schema_version=6;
ALTER TABLE release ADD CHECK (schema_version=6);
ALTER TABLE document DROP CONSTRAINT document_text_source_check;
ALTER TABLE document ADD CHECK (text_source IN ('source_tree','html_transcription','ocr','none'));
ALTER TABLE taxonomy_assignment DROP CONSTRAINT taxonomy_assignment_method_check;
ALTER TABLE taxonomy_assignment ADD CHECK (method IN ('manual','phapdien_seed','llm'))`)
			} else {
				textSource = "pdf_text"
				if version == 7 {
					readExec(t, db, `DROP TABLE document_summary;
ALTER TABLE release DROP CONSTRAINT release_schema_version_check;
UPDATE release SET schema_version=7;
ALTER TABLE release ADD CHECK (schema_version=7)`)
				}
				readExec(t, db, `UPDATE document SET text_source='pdf_text',text_complete=false WHERE id=1;
INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES
(1,'vn',NULL,1,'1','root','','custom','active','n1',1,3,1),
(2,'vn',1,2,'1.1','family','','custom','active','n1.n2',1,3,1);
INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES (11,2,2,'inherited',false,0.9,'v1')`)
			}
			engine, err := Open(ctx, db, embedder, fixtureOptions())
			if err != nil {
				t.Fatal(err)
			}
			if engine.Release().SchemaVersion != version {
				t.Fatal(engine.Release().SchemaVersion)
			}
			read, err := engine.Read(ctx, ReadRequest{Release: "test", Items: []ReadItem{{Handle: "vn:1"}}})
			if err != nil || len(read.Items) != 1 || read.Items[0].Error != "" || read.Items[0].Document == nil || read.Items[0].Document.TextSource != textSource {
				t.Fatalf("read = %+v, error = %v", read, err)
			}
			if version >= 7 && !reflect.DeepEqual(read.Items[0].Document.Nodes, []string{"1.1"}) {
				t.Fatal(read.Items[0].Document.Nodes)
			}
			response, err := engine.Search(ctx, Request{Query: "ngân hàng", Mode: "exact"})
			if err != nil || len(response.Hits) == 0 {
				t.Fatalf("search = %+v, error = %v", response, err)
			}
			presented, err := engine.Present(ctx, response, DetailFull)
			if err != nil || presented.Documents["vn:1"].TextSource != textSource {
				t.Fatalf("documents = %+v, error = %v", presented.Documents, err)
			}
		})
	}
}

func TestEffectiveNodes_InheritedPrecedence(t *testing.T) {
	db, engine, _ := fixture(t)
	readExec(t, db, `INSERT INTO taxonomy_node(id,country,parent_id,level,code,name,description,origin,status,path,ordinal,provision_count,document_count) VALUES
(1,'vn',NULL,1,'1','root','','custom','active','n1',1,3,1),
(2,'vn',1,2,'1.1','family','','custom','active','n1.n2',1,3,1),
(3,'vn',2,3,'1.1.inherited','inherited','','custom','active','n1.n2.n3',1,3,1),
(4,'vn',2,3,'1.1.llm','llm','','custom','active','n1.n2.n4',2,3,1),
(5,'vn',2,3,'1.1.manual','manual','','custom','active','n1.n2.n5',3,3,1),
(6,'vn',2,3,'1.1.seed','seed','','custom','active','n1.n2.n6',4,3,1);
INSERT INTO taxonomy_assignment(provision_id,node_id,family_id,method,exclude,confidence,taxonomy_version) VALUES
(11,3,2,'inherited',false,0.9,'v1'),(11,4,2,'llm',false,0.9,'v1')`)
	for _, tc := range []struct {
		name, sql, node string
	}{
		{"inherited over llm", "", "1.1.inherited"},
		{"seed over inherited", `INSERT INTO taxonomy_assignment VALUES(11,6,2,'phapdien_seed',false,1,'v1')`, "1.1.seed"},
		{"manual over seed", `INSERT INTO taxonomy_assignment VALUES(11,5,2,'manual',false,1,'v1')`, "1.1.manual"},
		{"nearest inherited over ancestor manual", `INSERT INTO taxonomy_assignment VALUES(12,3,2,'inherited',false,0.9,'v1')`, "1.1.inherited"},
		{"nearest exclusion", `INSERT INTO taxonomy_assignment VALUES(13,2,2,'manual',true,1,'v1')`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.sql != "" {
				readExec(t, db, tc.sql)
			}
			tx, err := db.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			nodes, _, err := engine.loadEffectiveNodes(t.Context(), tx, []int64{13})
			if rollbackErr := tx.Rollback(t.Context()); rollbackErr != nil {
				t.Fatal(rollbackErr)
			}
			if err != nil || strings.Join(nodes[13], ",") != tc.node {
				t.Fatalf("nodes = %v, error = %v", nodes, err)
			}
		})
	}
}

func TestDocuments_SourceFilesOrderedAndNullable(t *testing.T) {
	db, engine := readFixture(t)
	readExec(t, db, `INSERT INTO source_file(id,document_id,source,name,url,hash,size,role,fetched_at) VALUES
(1,1,'vbpl','z.pdf',NULL,repeat('c',64),3,'unknown',NULL),
(2,1,'vbpl','a.pdf','https://example.invalid/a',repeat('b',64),2,'main','2099-01-01T00:00:00Z'),
(3,1,'vbpl','a.pdf',NULL,repeat('a',64),1,'scan',NULL),
(4,2,'vbpl','other.pdf',NULL,repeat('d',64),4,'appendix',NULL)`)
	result, err := engine.Read(t.Context(), ReadRequest{Release: engine.release.Rel, Items: []ReadItem{{Handle: "vn:1"}}, WholeDocument: true, DocumentKey: "vn:1", PageCap: 2_000_000, PartSize: 1_000_000})
	if err != nil || len(result.Items) != 1 {
		t.Fatalf("read = %+v, error = %v", result, err)
	}
	files := result.Items[0].Files
	if len(files) != 3 || files[0].Name != "a.pdf" || files[0].Hash != strings.Repeat("a", 64) || files[1].Hash != strings.Repeat("b", 64) || files[2].Name != "z.pdf" || files[0].URL != nil || files[0].FetchedAt != nil || files[1].URL == nil || *files[1].URL != "https://example.invalid/a" || files[1].FetchedAt == nil || files[1].Size != 2 || files[1].Role != "main" {
		t.Fatalf("files = %+v", files)
	}
	raw, err := json.Marshal(result)
	if err != nil || strings.Contains(string(raw), "files") || strings.Contains(string(raw), "fetched_at") {
		t.Fatalf("MCP result = %s, error = %v", raw, err)
	}
}
