package build

import (
	"context"
	"regexp"
	"strings"
	"testing"

	schema "danny.vn/trachuan/internal/corpusdb/frameworks"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

type gapSchemaCapture struct{ sql string }

func (c *gapSchemaCapture) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	c.sql += sql
	return pgconn.CommandTag{}, nil
}

func embeddedGapSchema(t *testing.T) string {
	t.Helper()
	capture := &gapSchemaCapture{}
	require.NoError(t, schema.ApplyTables(context.Background(), capture, 1024))
	return capture.sql
}

func TestGapKinds_MatchesEmbeddedSchema(t *testing.T) {
	require.Equal(t, gapKinds(), schemaGapKinds(t, embeddedGapSchema(t)))
}

func TestGapSchemaParser_DetectsConstraintDrift(t *testing.T) {
	sql := embeddedGapSchema(t)
	for _, change := range []struct{ old, new string }{
		{"'mapping_dropped'", "'extra_kind'"},
		{"class='public'", "class='other'"},
		{"kind IN ('ocr_blocked','title_missing')", "kind IN ('ocr_blocked')"},
		{"kind <> 'ocr_blocked' OR blocks", "kind <> 'title_missing' OR blocks"},
	} {
		t.Run(change.new, func(t *testing.T) {
			changed := strings.Replace(sql, change.old, change.new, 1)
			require.NotEqual(t, sql, changed)
			require.NotEqual(t, gapKinds(), schemaGapKinds(t, changed))
		})
	}
}

func schemaGapKinds(t *testing.T, sql string) map[string]gapRule {
	t.Helper()
	classes := sqlCheck(t, sql, "quality_gap_class_check")
	branch := regexp.MustCompile(`\(\s*class\s*=\s*'([^']+)'\s+AND\s+kind\s+IN\s*\(([^)]*)\)\s*\)`)
	matches := branch.FindAllStringSubmatch(classes, -1)
	require.NotEmpty(t, matches)
	require.Equal(t, strings.Repeat("OR", len(matches)-1), strings.Join(strings.Fields(branch.ReplaceAllString(classes, "")), ""), "unsupported class constraint")
	kinds := map[string]gapRule{}
	for _, match := range matches {
		for _, kind := range sqlKindList(t, match[2]) {
			require.NotContains(t, kinds, kind)
			kinds[kind] = gapRule{Class: match[1], Blocking: "never"}
		}
	}
	blocks := sqlCheck(t, sql, "quality_gap_blocks_check")
	blocking := regexp.MustCompile(`^\s*\(\s*kind\s*<>\s*'([^']+)'\s+OR\s+blocks\s*\)\s+AND\s+\(\s*NOT\s+blocks\s+OR\s+kind\s+IN\s*\(([^)]*)\)\s*\)\s*$`).FindStringSubmatch(blocks)
	require.Len(t, blocking, 3, "unsupported blocking constraint")
	allowed := sqlKindList(t, blocking[2])
	require.Contains(t, allowed, blocking[1])
	for _, kind := range allowed {
		rule, ok := kinds[kind]
		require.True(t, ok, "blocking kind absent from class constraint")
		rule.Blocking = "licensed"
		if kind == blocking[1] {
			rule.Blocking = "always"
		}
		kinds[kind] = rule
	}
	return kinds
}

func sqlKindList(t *testing.T, list string) []string {
	t.Helper()
	var kinds []string
	for _, item := range strings.Split(list, ",") {
		item = strings.TrimSpace(item)
		require.Regexp(t, `^'[a-z][a-z0-9_]*'$`, item)
		kind := strings.Trim(item, "'")
		require.NotContains(t, kinds, kind)
		kinds = append(kinds, kind)
	}
	return kinds
}

func sqlCheck(t *testing.T, sql, name string) string {
	t.Helper()
	location := regexp.MustCompile(`\bCONSTRAINT\s+` + regexp.QuoteMeta(name) + `\s+CHECK\s*\(`).FindStringIndex(sql)
	require.Len(t, location, 2, "constraint missing")
	start := location[1]
	depth, quoted := 1, false
	for i := start; i < len(sql); i++ {
		if sql[i] == '\'' {
			if quoted && i+1 < len(sql) && sql[i+1] == '\'' {
				i++
				continue
			}
			quoted = !quoted
			continue
		}
		if quoted {
			continue
		}
		switch sql[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return sql[start:i]
			}
		}
	}
	t.Fatal("unterminated constraint")
	return ""
}
