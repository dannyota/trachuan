package serve

import (
	"testing"

	"github.com/stretchr/testify/require"
)

type schemaRelation struct {
	name, kind   string
	dependencies []string
}

func classifyRelations(relations []schemaRelation) map[string]relationClass {
	publicTables := map[string]bool{
		"framework": true, "framework_version": true, "control": true,
		"control_mapping": true, "public_chunk": true, "public_posting": true,
	}
	byName := map[string]schemaRelation{}
	for _, relation := range relations {
		byName[relation.name] = relation
	}
	classes := map[string]relationClass{}
	visiting := map[string]bool{}
	var classify func(string) relationClass
	classify = func(name string) relationClass {
		if class, ok := classes[name]; ok {
			return class
		}
		relation, known := byName[name]
		if !known || visiting[name] {
			return textRelation
		}
		class := textRelation
		switch relation.kind {
		case "r", "p":
			if publicTables[name] {
				class = publicRelation
			} else if name == "release" || name == "quality_gap" {
				class = mixedRelation
			}
		case "v", "m":
			visiting[name] = true
			if len(relation.dependencies) > 0 {
				class = publicRelation
				for _, dependency := range relation.dependencies {
					if classify(dependency) != publicRelation {
						class = textRelation
						break
					}
				}
			}
			delete(visiting, name)
		}
		classes[name] = class
		return class
	}
	for _, relation := range relations {
		classify(relation.name)
	}
	return classes
}

func TestProjectionSchema_DerivesTextViewsAndClosesUnknownRelations(t *testing.T) {
	classes := classifyRelations([]schemaRelation{
		{name: "body_chunk", kind: "r"}, {name: "control_text", kind: "r"},
		{name: "body_posting", kind: "r"}, {name: "new_private_table", kind: "r"},
		{name: "public_chunk", kind: "r"}, {name: "public_posting", kind: "r"},
		{name: "quality_gap", kind: "r"}, {name: "release", kind: "r"},
		{name: "body_lexical_stats", kind: "v", dependencies: []string{"body_chunk"}},
		{name: "body_lexical_df", kind: "v", dependencies: []string{"body_posting"}},
		{name: "public_lexical_stats", kind: "v", dependencies: []string{"public_chunk"}},
		{name: "public_lexical_df", kind: "v", dependencies: []string{"public_posting"}},
		{name: "invented_summary", kind: "v", dependencies: []string{"control_text", "public_chunk"}},
		{name: "invented_rollup", kind: "m", dependencies: []string{"invented_summary"}},
		{name: "invented_missing", kind: "v", dependencies: []string{"missing"}},
		{name: "invented_external", kind: "v", dependencies: []string{"private.public_chunk"}},
		{name: "invented_cycle", kind: "v", dependencies: []string{"invented_cycle"}},
	})
	for _, name := range []string{"body_chunk", "body_posting", "control_text", "new_private_table", "body_lexical_stats", "body_lexical_df", "invented_summary", "invented_rollup", "invented_missing", "invented_external", "invented_cycle"} {
		require.Equal(t, textRelation, classes[name], name)
	}
	require.Equal(t, publicRelation, classes["public_lexical_stats"])
	require.Equal(t, publicRelation, classes["public_lexical_df"])
	require.Equal(t, mixedRelation, classes["quality_gap"])
	require.Equal(t, mixedRelation, classes["release"])
}
