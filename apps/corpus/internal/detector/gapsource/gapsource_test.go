package gapsource

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"slices"
	"testing"
)

func TestNames_EnumeratesGapSources(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", `package synthetic
const namedGap = "constant_gap"
func reason() (int,string) { return 0,"returned_gap" }
func record() {
 gaps = append(gaps, Gap{Kind:"literal_gap"}, Gap{Kind:namedGap})
 gaps = []Gap{{Kind:"nested_gap"}}
 selected = selection{gap:"selected_gap"}
 _,gap := reason()
 selected.extraGap = "extra_gap"
 l.addGap("build",key,"build_gap","")
 report.addGap("report_gap","","")
 name := "initial_gap"
 if alternate { name = "alternate_gap" }
 report.addGap(name,"","")
 report.addGap(externalGap,"","")
 l.taxonomyGap(key,"taxonomy_gap","",1)
 stage.RetryableGap(err,"retry_gap")
 result.Gaps = append(result.Gaps,"parsed_gap")
 rejected = Rejected{1,nil,"embedding_gap"}
 value = Relation{Kind:"not_a_gap"}
}`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	external, err := parser.ParseFile(token.NewFileSet(), "external.go", `package synthetic; const externalGap = "external_gap"`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	got := Names(map[string]*ast.File{"synthetic.go": file, "external.go": external})
	want := []string{"alternate_gap", "build_gap", "constant_gap", "embedding_gap", "external_gap", "extra_gap", "initial_gap", "literal_gap", "nested_gap", "parsed_gap", "report_gap", "retry_gap", "returned_gap", "selected_gap", "taxonomy_gap"}
	if !slices.Equal(got, want) {
		t.Fatalf("gap names=%v, want %v", got, want)
	}
}

func TestSources_AttributesKindsToDirectories(t *testing.T) {
	parse := func(name, text string) *ast.File {
		file, err := parser.ParseFile(token.NewFileSet(), name, text, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		return file
	}
	files := map[string]*ast.File{
		"a/one/x.go": parse("a/one/x.go", `package one; const shared = "shared_gap"; func f() { gaps = append(gaps, Gap{Kind: "one_gap"}, Gap{Kind: shared}) }`),
		"a/two/y.go": parse("a/two/y.go", `package two; func g() { gaps = append(gaps, Gap{Kind: "shared_gap"}) }`),
	}
	got := Sources(files)
	want := map[string][]string{"one_gap": {"a/one"}, "shared_gap": {"a/one", "a/two"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sources=%v, want %v", got, want)
	}
}

func TestNames_FieldAssignmentsToLowercaseKind(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", `package synthetic
func decide() {
 out.kind = "assigned_kind"
 out.Kind = "exported_kind"
 var plain string
 plain = "not_a_gap"
}`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	got := Names(map[string]*ast.File{"synthetic.go": file})
	want := []string{"assigned_kind"}
	if !slices.Equal(got, want) {
		t.Fatalf("gap names=%v, want %v", got, want)
	}
}

func TestNames_KindAssignmentsOnGapBindings(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", `package synthetic
func failure() stage.Gap {
 g := stage.Gap{Kind: "initial_gap"}
 g.Kind = "assigned_gap"
 d := rule{Kind: "rule_kind"}
 d.Kind = "keyword"
 rows[i].Kind = "indexed_kind"
 return g
}`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	got := Names(map[string]*ast.File{"synthetic.go": file})
	want := []string{"assigned_gap", "initial_gap"}
	if !slices.Equal(got, want) {
		t.Fatalf("gap names=%v, want %v", got, want)
	}
}
