//go:build integration

package frameworks

import (
	"strings"
	"testing"
)

func TestSchema_RejectsInvalidGapKindsAndBlocking(t *testing.T) {
	conn := schemaDB(t)
	seed(t, conn)
	cases := map[string]violationCase{
		"ocr must block":      {`INSERT INTO quality_gap VALUES('ocr_blocked','text',1,'A.2','1',false)`, "23514", "quality_gap_blocks_check"},
		"public cannot block": {`INSERT INTO quality_gap VALUES('mapping_dropped','public',1,'A.2','1',true)`, "23514", "quality_gap_blocks_check"},
		"text cannot block":   {`INSERT INTO quality_gap VALUES('ocr_flagged','text',1,'A.2','1',true)`, "23514", "quality_gap_blocks_check"},
		"blocks required":     {`INSERT INTO quality_gap(kind,class,version_id,subject,detail) VALUES('ocr_flagged','text',1,'A.2','1')`, "23502", ""},
	}
	for _, kind := range []string{"control_count", "lineage_invalid", "parse", "extraction", "ocr", "completeness", "ocr_quote_not_found", "ocr_rejected", "ocr_label_flagged"} {
		cases[kind] = violationCase{`UPDATE quality_gap SET blocks=false,kind='` + kind + `'`, "23514", "quality_gap_class_check"}
	}
	runViolations(t, conn, cases)
}

func TestSchema_StepRecipesAndInputHashes(t *testing.T) {
	conn := schemaDB(t)
	seed(t, conn)
	cases := map[string]violationCase{}
	for _, step := range []string{"snapshot", "parse", "relate", "chunk", "embed"} {
		for _, value := range []string{"[]", "null", "42", `"recipe"`} {
			cases[step+" recipe "+value] = violationCase{`UPDATE release SET ` + step + `_recipe='` + value + `'`, "23514", "release_" + step + "_recipe_check"}
		}
		cases[step+" recipe required"] = violationCase{`UPDATE release SET ` + step + `_recipe=NULL`, "23502", ""}
		cases[step+" hash required"] = violationCase{`UPDATE release SET ` + step + `_input_hash=NULL`, "23502", ""}
		for _, value := range []string{"", "a", strings.Repeat("a", 63), strings.Repeat("a", 65), strings.Repeat("A", 64), strings.Repeat("g", 64), strings.Repeat("a", 64) + "\n"} {
			cases[step+" hash "+value] = violationCase{`UPDATE release SET ` + step + `_input_hash='` + value + `'`, "23514", "release_" + step + "_input_hash_check"}
		}
	}
	runViolations(t, conn, cases)
}
