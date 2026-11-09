package retrieval

import "testing"

func TestShapeRelations_KeepsIdenticalArtifactsFromTwoSources(t *testing.T) {
	target := "vn:vbpl:3"
	edge := func(source string) SourceRelation {
		return SourceRelation{Value: Relation{SourceKey: source, TargetKey: &target, Kind: "cites", Evidence: "x"}, ArtifactSHA256: "aa", Ordinal: 0}
	}
	got, gaps := shapeRelations([]SourceRelation{edge("vn:vbpl:2"), edge("vn:vbpl:1")}, Recipe{RelationCap: 10, RelationEvidenceRuneCap: 100}, Identity{})
	if len(got) != 2 || len(gaps) != 0 || got[0].SourceKey != "vn:vbpl:1" || got[1].SourceKey != "vn:vbpl:2" {
		t.Fatalf("relations %+v gaps %+v", got, gaps)
	}
}

func TestShapeRelations_DropsTheSameEdgeTwice(t *testing.T) {
	target := "vn:vbpl:3"
	e := SourceRelation{Value: Relation{SourceKey: "vn:vbpl:1", TargetKey: &target, Kind: "cites", Evidence: "x"}, ArtifactSHA256: "aa", Ordinal: 0}
	got, _ := shapeRelations([]SourceRelation{e, e}, Recipe{RelationCap: 10, RelationEvidenceRuneCap: 100}, Identity{})
	if len(got) != 1 {
		t.Fatalf("relations %+v", got)
	}
}
