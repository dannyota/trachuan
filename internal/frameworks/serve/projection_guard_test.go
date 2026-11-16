package serve

import (
	"fmt"
	"regexp"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"
)

type relationClass string

const (
	publicRelation relationClass = "public"
	textRelation   relationClass = "text"
	mixedRelation  relationClass = "mixed"
)

type queryRead struct {
	sql                      string
	versions, textGaps       []int64
	hasVersions, hasTextGaps bool
}

func checkProjection(reads []queryRead, classes map[string]relationClass, readable []int64) error {
	rules := projectionRules()
	for _, read := range reads {
		rule, classified := rules[normalizeSQL(read.sql)]
		if !classified {
			return fmt.Errorf("unclassified statement")
		}
		if err := checkRuleSources(rule, classes); err != nil {
			return err
		}
		if rule.kind == "projected" {
			if !read.hasVersions {
				return fmt.Errorf("private read without a bound version set")
			}
			for _, id := range read.versions {
				if !slices.Contains(readable, id) {
					return fmt.Errorf("private structure read for an unreadable version")
				}
			}
		}
		if rule.kind == "gaps" {
			if !read.hasVersions || !read.hasTextGaps {
				return fmt.Errorf("gap read without bound projection sets")
			}
			for _, id := range read.textGaps {
				if !slices.Contains(readable, id) {
					return fmt.Errorf("text gap read for an unreadable version")
				}
			}
		}
	}
	return nil
}

func checkRuleSources(rule statementRule, classes map[string]relationClass) error {
	sources := regexp.MustCompile(`\b(?:FROM|JOIN) ([a-z_][a-z0-9_]*)\b`).FindAllStringSubmatch(normalizeSQL(rule.sql), -1)
	for _, source := range sources {
		name := source[1]
		if rule.kind == "catalog" && name == "pg_attribute" {
			continue
		}
		switch classes[name] {
		case publicRelation:
		case textRelation:
			if rule.kind != "projected" {
				return fmt.Errorf("unfiltered text-derived relation %q", name)
			}
		case mixedRelation:
			if (name != "release" || rule.kind != "release") && (name != "quality_gap" || rule.kind != "gaps") {
				return fmt.Errorf("unfiltered mixed relation %q", name)
			}
		default:
			return fmt.Errorf("unclassified schema relation %q", name)
		}
	}
	return nil
}

func TestProjectionGuard_RejectsDiscardedAndUnclassifiedReads(t *testing.T) {
	for _, sql := range []string{
		`SELECT count(*) FROM body_lexical_df`,
		`SELECT count(*) FROM body_lexical_stats`,
		`SELECT count(*) FROM mapping_evidence`,
		`SELECT count(*) FROM source_file`,
		`SELECT count(*) FROM invented_body_summary`,
		`SELECT body FROM control_text`,
		`SELECT count(*) FROM unclassified_relation`,
		`SELECT 1`,
		`SELECT id,version_id,control_id,ordinal,token_count,1-(embedding <=> $2::vector) FROM body_chunk WHERE version_id=ANY($1::bigint[]) OR true`,
		`SELECT chunk_id,token,tf FROM body_posting WHERE version_id=ANY($1::bigint[]); SELECT count(*) FROM body_lexical_df`,
	} {
		t.Run(sql, func(t *testing.T) {
			read := queryRead{sql: sql, versions: []int64{3}, hasVersions: true}
			require.Error(t, checkProjection([]queryRead{read}, nil, []int64{3}))
		})
	}
}

func TestProjectionGuard_EnforcesBoundVersionsAndSchemaClasses(t *testing.T) {
	classes := map[string]relationClass{"body_chunk": textRelation, "public_lexical_stats": publicRelation}
	body := queryRead{sql: `SELECT id,version_id,control_id,ordinal,token_count,1-(embedding <=> $2::vector) FROM body_chunk WHERE version_id=ANY($1::bigint[])`, versions: []int64{3}, hasVersions: true}
	require.NoError(t, checkProjection([]queryRead{body}, classes, []int64{3}))
	require.Error(t, checkProjection([]queryRead{body}, classes, nil))
	body.hasVersions = false
	require.Error(t, checkProjection([]queryRead{body}, classes, []int64{3}))
	public := queryRead{sql: `SELECT chunk_count,avg_length FROM public_lexical_stats`}
	require.NoError(t, checkProjection([]queryRead{public}, classes, nil))
	classes["public_lexical_stats"] = textRelation
	require.ErrorContains(t, checkProjection([]queryRead{public}, classes, nil), "unfiltered text-derived relation")
	require.Error(t, checkProjection([]queryRead{public}, nil, nil))
}

func TestProjectionGuard_AuditsMixedGapBranches(t *testing.T) {
	classes := map[string]relationClass{"quality_gap": mixedRelation, "framework_version": publicRelation}
	read := queryRead{
		sql: `SELECT g.kind,'fw:'||v.framework_code||':'||v.version,g.subject,g.detail
 FROM quality_gap g JOIN framework_version v ON v.id=g.version_id
 WHERE g.version_id=ANY($1::bigint[]) AND (g.class='public' OR (g.class='text' AND g.version_id=ANY($2::bigint[])))
 ORDER BY g.kind COLLATE "C",v.framework_code COLLATE "C",v.version COLLATE "C",g.subject COLLATE "C",g.detail COLLATE "C"`,
		versions: []int64{1, 2, 3}, hasVersions: true, hasTextGaps: true,
	}
	require.NoError(t, checkProjection([]queryRead{read}, classes, nil))
	read.textGaps = []int64{3}
	require.NoError(t, checkProjection([]queryRead{read}, classes, []int64{3}))
	require.Error(t, checkProjection([]queryRead{read}, classes, nil))
	read.hasTextGaps = false
	require.Error(t, checkProjection([]queryRead{read}, classes, []int64{3}))
}
