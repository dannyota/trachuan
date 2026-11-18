package evidence

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/detector"
)

func extractDir(t *testing.T, text string) (string, string) {
	t.Helper()
	root := t.TempDir()
	store := artifact.Store{Root: root}
	res, err := store.Publish(context.Background(), "extract", hash64("a"), hash64("b"), func(dir string) error {
		return os.WriteFile(filepath.Join(dir, "text.json"), []byte(text), 0600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return store.Abs(res.Path), res.SHA256
}

func hash64(c string) string {
	out := ""
	for len(out) < 64 {
		out += c
	}
	return out
}

func waivedInputs(dirs map[string]string) func(string, string) (string, bool, error) {
	return func(stage, key string) (string, bool, error) {
		dir, ok := dirs[key]
		return dir, ok, nil
	}
}

func TestWaivedDocumentWithoutRelations_Fires(t *testing.T) {
	waived, waivedSum := extractDir(t, `{"text_source":"ocr","waiver":{"key":"vn:1","check_sha256":"`+hash64("c")+`"}}`)
	rel := &fakeRelease{documents: []releaseDocument{
		{ID: 1, Key: "vn:1", TextSource: "ocr", ExtractSHA256: waivedSum},
		{ID: 2, Key: "vn:2", TextSource: "ocr", ExtractSHA256: waivedSum, TextRelation: true},
		{ID: 3, Key: "vn:3", TextSource: "source_tree"},
		{ID: 4, Key: "vn:4", TextSource: "ocr", ExtractSHA256: waivedSum, Baseline: true},
	}}
	obs, err := detectWaived(context.Background(), rel, waivedInputs(map[string]string{"vn:1": waived, "vn:2": waived}))
	if err != nil {
		t.Fatal(err)
	}
	if len(obs.Findings) != 1 || obs.Findings[0].Subject.Fields[0] != "vn:1" || obs.Observed != 2 || obs.NotObservable != 1 {
		t.Fatalf("%+v", obs)
	}
}

func TestWaivedDocumentWithoutRelations_UnwaivedOCRDocumentIsClean(t *testing.T) {
	dir, sum := extractDir(t, `{"text_source":"ocr"}`)
	rel := &fakeRelease{documents: []releaseDocument{{ID: 1, Key: "vn:1", TextSource: "ocr", ExtractSHA256: sum}}}
	obs, err := detectWaived(context.Background(), rel, waivedInputs(map[string]string{"vn:1": dir}))
	if err != nil || len(obs.Findings) != 0 {
		t.Fatalf("%v %+v", err, obs)
	}
}

func TestWaivedDocumentWithoutRelations_MissingOrChangedInputFails(t *testing.T) {
	dir, sum := extractDir(t, `{"waiver":{"key":"vn:1","check_sha256":"x"}}`)
	doc := releaseDocument{ID: 1, Key: "vn:1", TextSource: "ocr", ExtractSHA256: sum}
	for name, in := range map[string]func(string, string) (string, bool, error){
		"no artifact": waivedInputs(nil),
		"wrong hash":  waivedInputs(map[string]string{"vn:1": dir}),
	} {
		d := doc
		if name == "wrong hash" {
			d.ExtractSHA256 = hash64("e")
		}
		if _, err := detectWaived(context.Background(), &fakeRelease{documents: []releaseDocument{d}}, in); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
	if _, err := detectWaived(context.Background(), &fakeRelease{documents: []releaseDocument{doc}}, nil); err == nil {
		t.Error("nil artifact lookup: no error")
	}
}

func TestWaivedDocumentWithoutRelations_NeverReadSourcesAreNotObservable(t *testing.T) {
	waived, sum := extractDir(t, `{"waiver":{"key":"x","check_sha256":"`+hash64("c")+`"}}`)
	rel := &fakeRelease{documents: []releaseDocument{
		{ID: 1, Key: "vn:chinhphu:1", Source: "chinhphu", TextSource: "ocr", ExtractSHA256: sum},
		{ID: 2, Key: "vn:2", Consolidated: true, TextSource: "ocr", ExtractSHA256: sum},
		{ID: 3, Key: "vn:3", TextSource: "ocr", ExtractSHA256: sum},
	}}
	dirs := waivedInputs(map[string]string{"vn:3": waived})
	obs, err := detectWaived(context.Background(), rel, dirs)
	if err != nil || len(obs.Findings) != 1 || obs.Findings[0].Subject.Fields[0] != "vn:3" || obs.Observed != 1 || obs.NotObservable != 2 {
		t.Fatalf("%v %+v", err, obs)
	}
	if obs.ObservedCompletely(docSubject("vn:1")) || obs.ObservedCompletely(docSubject("vn:2")) {
		t.Fatal("never-read source counts as observed")
	}
}

func TestWaivedDocumentWithoutRelations_LostInputCarriesThePreviousFinding(t *testing.T) {
	waived, sum := extractDir(t, `{"waiver":{"key":"x","check_sha256":"`+hash64("c")+`"}}`)
	doc := releaseDocument{ID: 1, Key: "vn:1", TextSource: "ocr", ExtractSHA256: sum}
	dirs := waivedInputs(map[string]string{"vn:1": waived})
	file := func(rel string, previous *File, d releaseDocument, gaps map[string]map[string]bool) File {
		obs, err := detectWaived(context.Background(), &fakeRelease{documents: []releaseDocument{d}, gaps: gaps}, dirs)
		if err != nil {
			t.Fatal(err)
		}
		r := run(detector.WaivedDocumentWithoutRelations, 1, obs.ObservedCompletely, obs.Findings...)
		return mustCompute(t, computeInput{Rel: rel, Runs: []detectorRun{r}, Previous: previous})
	}
	first := file("20990101_1", nil, doc, map[string]map[string]bool{"vn:1": {"ocr_flagged": true}})
	if len(first.Findings) != 1 {
		t.Fatalf("first %+v", first.Findings)
	}
	related := doc
	related.TextRelation = true
	if second := file("20990102_1", &first, related, map[string]map[string]bool{"vn:1": {"ocr_flagged": true}}); len(second.Gone) != 1 {
		t.Fatalf("relation gained: gone %v carried %v", second.Gone, second.Carried)
	}
	noWaiver, noWaiverSum := extractDir(t, `{"text_source":"ocr"}`)
	cleared := doc
	cleared.ExtractSHA256 = noWaiverSum
	dirs = waivedInputs(map[string]string{"vn:1": noWaiver})
	if second := file("20990102_1", &first, cleared, map[string]map[string]bool{"vn:1": {"ocr_flagged": true}}); len(second.Gone) != 1 {
		t.Fatalf("waiver removed: gone %v carried %v", second.Gone, second.Carried)
	}
	for _, source := range []string{"source_tree", "html_transcription", "pdf_text"} {
		changed := doc
		changed.TextSource = source
		if second := file("20990102_1", &first, changed, nil); len(second.Gone) != 1 || len(second.Carried) != 0 {
			t.Fatalf("%s text: gone %v carried %v", source, second.Gone, second.Carried)
		}
	}
	for name, change := range map[string]struct {
		mutate func(*releaseDocument)
		gaps   map[string]map[string]bool
	}{
		"extract gap":        {func(*releaseDocument) {}, map[string]map[string]bool{"vn:1": {"ocr_flagged": true, "ocr_waiver_stale": true}}},
		"no text at all now": {func(d *releaseDocument) { d.TextSource = "none" }, nil},
		"needs ocr":          {func(*releaseDocument) {}, map[string]map[string]bool{"vn:1": {"needs_ocr": true}}},
	} {
		t.Run(name, func(t *testing.T) {
			d := cleared
			change.mutate(&d)
			dirs = waivedInputs(map[string]string{"vn:1": noWaiver})
			if second := file("20990102_1", &first, d, change.gaps); len(second.Gone) != 0 || len(second.Carried) != 1 {
				t.Fatalf("gone %v carried %v", second.Gone, second.Carried)
			}
		})
	}
}

func TestWaivedDocumentWithoutRelations_CountsOnlyOCRDocuments(t *testing.T) {
	rel := &fakeRelease{documents: []releaseDocument{
		{ID: 1, Key: "vn:1", TextSource: "source_tree"},
		{ID: 2, Key: "vn:2", TextSource: "source_tree", Baseline: true},
		{ID: 3, Key: "vn:3", TextSource: "none"},
		{ID: 4, Key: "vn:4", TextSource: "ocr", Baseline: true},
	}}
	obs, err := detectWaived(context.Background(), rel, nil)
	if err != nil || obs.Observed != 0 || obs.NotObservable != 1 {
		t.Fatalf("%v %+v", err, obs)
	}
	if !obs.ObservedCompletely(docSubject("vn:1")) || obs.ObservedCompletely(docSubject("vn:3")) || obs.ObservedCompletely(docSubject("vn:2")) {
		t.Fatal("coverage is wrong")
	}
}
