package classify

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/batchapi"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func TestSelectLabelDocuments_DistinctSortedPrimaryKeys(t *testing.T) {
	labels := []taxonomy.LabelArticle{{Key: "vn:vbpl:2"}, {Key: "vn:vbpl:1"}, {Key: "vn:vbpl:2"}}
	primary := []Document{{Key: "vn:vbpl:3"}, {Key: "vn:vbpl:2"}, {Key: "vn:vbpl:1"}}
	selected, err := SelectLabelDocuments(labels, primary)
	if err != nil || len(selected) != 2 || selected[0].Key != "vn:vbpl:1" || selected[1].Key != "vn:vbpl:2" {
		t.Fatalf("selected %+v error %v", selected, err)
	}
	if primary[0].Key != "vn:vbpl:3" || labels[0].Key != "vn:vbpl:2" {
		t.Fatal("selection mutated its inputs")
	}
}

func TestSelectLabelDocuments_RefusesAbsentOrNonPrimaryKey(t *testing.T) {
	for _, key := range []string{"vn:vbpl:absent", "vn:vbpl:non-primary"} {
		t.Run(key, func(t *testing.T) {
			if _, err := SelectLabelDocuments([]taxonomy.LabelArticle{{Key: "vn:vbpl:1"}, {Key: key}}, []Document{{Key: "vn:vbpl:1"}}); err == nil || !strings.Contains(err.Error(), key) {
				t.Fatalf("accepted ineligible key %s: %v", key, err)
			}
		})
	}
}

func TestScreenSelection_UsesScorerReleaseValidation(t *testing.T) {
	for _, reason := range []string{"own text", "inactive code", "absent anchor"} {
		for _, count := range []int{19, 20} {
			labels, families, artifact, articles, nodes := scoreFixture(t, count)
			switch reason {
			case "own text":
				labels.Articles[0].OwnTextSHA256 = "stale"
			case "inactive code":
				labels.Articles[0].Codes = []string{"1.1.old"}
			case "absent anchor":
				labels.Articles[0].Anchor = "missing"
			}
			_, scoreErr := Score(labels, families, artifact, articles, nodes)
			err := ValidateScreenSelection(labels, articles, nodes)
			if (err != nil) != (count == 19) || (err != nil) != (scoreErr != nil) {
				t.Fatalf("%s count %d validation %v scorer %v", reason, count, err, scoreErr)
			}
		}
	}
}

func TestScreenSelection_HashSeparatesDrawFileBytesAndKeys(t *testing.T) {
	_, _, _, drawOptions := screenFixture(t)
	o := drawOptions
	o.Documents, o.Seed = 0, 0
	o.Selection = &DocumentSelection{LabelsHash: strings.Repeat("a", 64), DocumentKeys: []string{"vn:vbpl:1"}}
	first := screenHash(o)
	if first == screenHash(drawOptions) {
		t.Fatal("selection collided with draw")
	}
	path := filepath.Join(t.TempDir(), "labels.json")
	for _, body := range []string{`{"country":"vn","release":"20990101","articles":[{"document_key":"vn:vbpl:1","anchor":"dieu-1","own_text_sha256":"synthetic","codes":["1.1"]}]}`, `{"country":"vn","release":"20990101","articles":[{"document_key":"vn:vbpl:1","anchor":"dieu-1","own_text_sha256":"synthetic","codes":["1.1"]}]} `} {
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
		_, hash, err := ReadLabels(path)
		if err != nil {
			t.Fatal(err)
		}
		o.Selection = &DocumentSelection{LabelsHash: hash, DocumentKeys: []string{"vn:vbpl:1"}}
		if screenHash(o) == first {
			t.Fatal("changed labels file replayed old identity")
		}
		first = screenHash(o)
	}
	o.Selection = &DocumentSelection{LabelsHash: o.Selection.LabelsHash, DocumentKeys: []string{"vn:vbpl:2"}}
	if screenHash(o) == first {
		t.Fatal("changed selected keys replayed old identity")
	}
}

func TestScreenSelection_PersistsDescriptorAndReplaysArtifact(t *testing.T) {
	r, ledger, client, o := screenFixture(t)
	o.Documents, o.Seed = 0, 0
	o.Selection = &DocumentSelection{LabelsHash: strings.Repeat("a", 64), DocumentKeys: []string{"vn:vbpl:1", "vn:vbpl:2"}}
	prepare := client.beforeCreate
	client.beforeCreate = func(requests []batchapi.Request) {
		prepare(requests)
		second := client.results[0]
		second.CustomID = requests[1].CustomID
		client.results = append(client.results, second)
	}
	docs := append(documents(), Document{Key: "vn:vbpl:2", Title: "Synthetic second"})
	if _, err := r.Screen(context.Background(), o, docs, families()); err != nil {
		t.Fatal(err)
	}
	if len(ledger.rows) != 1 || len(ledger.rows[0].Items) != 2 || ledger.rows[0].Items[0].Key != "vn:vbpl:1" || ledger.rows[0].Items[1].Key != "vn:vbpl:2" {
		t.Fatalf("selected items %+v", ledger.rows)
	}
	read := func(dir string) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join(dir, "families.json"))
		if err != nil {
			t.Fatal(err)
		}
		var f FamilyArtifact
		if err = json.Unmarshal(b, &f); err != nil || !reflect.DeepEqual(f.Selection, o.Selection) {
			t.Fatalf("selection artifact %s error %v", b, err)
		}
		if _, _, err = ReadFamilyArtifact(filepath.Join(dir, "families.json"), o.Recipe.TaxonomyHash, articleNodes()); err != nil {
			t.Fatal(err)
		}
		return b
	}
	first := read(o.Out)
	var persisted Submission
	b, err := json.Marshal(ledger.rows[0])
	if err != nil || json.Unmarshal(b, &persisted) != nil {
		t.Fatal("submission round trip failed", err)
	}
	ledger.rows = []Submission{persisted}
	before := client.counts + client.creates + client.gets + client.downloads
	o.Out = filepath.Join(t.TempDir(), "replayed")
	if _, err = r.Screen(context.Background(), o, nil, families()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, read(o.Out)) || before != client.counts+client.creates+client.gets+client.downloads {
		t.Fatal("replay changed artifact or called provider")
	}
}
