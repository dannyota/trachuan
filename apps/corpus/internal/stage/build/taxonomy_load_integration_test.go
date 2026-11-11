//go:build integration

package build

import (
	"context"
	"errors"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func taxonomyInputFixture() TaxonomyInput {
	return TaxonomyInput{version: "fixture", mapping: taxonomy.Mapping{Nodes: []taxonomy.MappedNode{
		{GUID: "s", Code: "1", Level: 1, Status: "active", Ordinal: 1, Name: "Subject"},
		{GUID: "d", Code: "1.1", ParentCode: "1", Level: 2, Status: "active", Ordinal: 1, Name: "Family"},
		{GUID: "a", Code: "1.1.c1", ParentCode: "1.1", Level: 3, Status: "active", Ordinal: 1, Name: "Chapter"},
		{GUID: "r", Code: "1.1.c2", ParentCode: "1.1", Level: 3, Status: "retired", Name: "Retired"},
	}}, custom: []taxonomy.CustomNode{{Code: "1.1.x1", ParentCode: "1.1", Name: "Custom", Description: "Custom description."}}}
}

func loadTaxonomyFixture(t *testing.T, f *loadFixture, input TaxonomyInput) Counts {
	t.Helper()
	c, err := Load(context.Background(), f.corpus, f.inventory(t), f.artifacts, f.recipes, nil, nil, f.asOf(), LoadOptions{Taxonomy: input})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestTaxonomy_NodesAndPaths(t *testing.T) {
	f := fixture(t)
	c := loadTaxonomyFixture(t, f, taxonomyInputFixture())
	if c.Taxonomy.Nodes != 5 {
		t.Fatalf("nodes=%d", c.Taxonomy.Nodes)
	}
	var active, custom, retired int
	var path, description, parentPath, packageDescription string
	err := f.corpus.QueryRow(context.Background(), `SELECT (SELECT ordinal FROM taxonomy_node WHERE code='1.1.c1'),(SELECT ordinal FROM taxonomy_node WHERE code='1.1.x1'),(SELECT ordinal FROM taxonomy_node WHERE code='1.1.c2'),(SELECT path::text FROM taxonomy_node WHERE code='1.1.x1'),(SELECT description FROM taxonomy_node WHERE code='1.1.x1'),(SELECT p.path::text FROM taxonomy_node c JOIN taxonomy_node p ON p.id=c.parent_id WHERE c.code='1.1.x1'),(SELECT description FROM taxonomy_node WHERE code='1.1.c1')`).Scan(&active, &custom, &retired, &path, &description, &parentPath, &packageDescription)
	if err != nil || active != 1 || custom != 2 || retired != 3 || !strings.HasPrefix(path, parentPath+".n") || description != "Custom description." || packageDescription != "" {
		t.Fatalf("ordinals %d %d %d path %q description %q: %v", active, custom, retired, path, description, err)
	}
}

func TestSeeds_MissingArticleGap(t *testing.T) {
	f := fixture(t)
	f.fetch(t, "vn:vbpl:1")
	input := taxonomyInputFixture()
	input.seeds = []taxonomySeed{{"1", "5", "1.1.c1"}, {"1", "5", "1.1.c1"}}
	c := loadTaxonomyFixture(t, f, input)
	var n int
	err := f.corpus.QueryRow(context.Background(), `SELECT count(*) FROM coverage_gap WHERE kind='seed_article_missing' AND document_key='vn:vbpl:1' AND node_id IS NOT NULL AND detail='1.1.c1 dieu-5'`).Scan(&n)
	if err != nil || n != 1 || c.Taxonomy.MissingArticles != 1 {
		t.Fatalf("gap=%d stats=%+v: %v", n, c.Taxonomy, err)
	}
}

func TestOverrides_TargetMissingGap(t *testing.T) {
	f := fixture(t)
	input := taxonomyInputFixture()
	input.overrides = []taxonomy.Override{{DocumentKey: "vn:vbpl:missing", Anchor: "dieu-1", Code: "1.1.c1"}}
	c := loadTaxonomyFixture(t, f, input)
	var n int
	err := f.corpus.QueryRow(context.Background(), `SELECT count(*) FROM coverage_gap WHERE kind='override_target_missing' AND document_key='vn:vbpl:missing' AND node_id IS NOT NULL AND detail='1.1.c1 dieu-1'`).Scan(&n)
	if err != nil || n != 1 || c.Taxonomy.MissingOverrideTargets != 1 {
		t.Fatalf("gap=%d stats=%+v: %v", n, c.Taxonomy, err)
	}
}

func TestOverrides_Precedence(t *testing.T) {
	f := fixture(t)
	addValidityFixture(t, f, "1", "1/2024/NĐ-CP", "CHL", "2024-01-01", "2024-01-01", "text")
	input := taxonomyInputFixture()
	input.mapping.Nodes = append(input.mapping.Nodes, taxonomy.MappedNode{GUID: "other", Code: "1.2", ParentCode: "1", Level: 2, Status: "active", Ordinal: 2, Name: "Other"})
	input.seeds = []taxonomySeed{{"1", "1", "1.1.c1"}, {"1", "1", "1.2"}}
	input.overrides = []taxonomy.Override{{DocumentKey: "vn:vbpl:1", Anchor: "dieu-1", Code: "1.1.x1", Exclude: true}}
	c := loadTaxonomyFixture(t, f, input)
	var method string
	var excluded bool
	err := f.corpus.QueryRow(context.Background(), `SELECT method,exclude FROM taxonomy_assignment WHERE method='manual'`).Scan(&method, &excluded)
	if err != nil || method != "manual" || !excluded || c.Taxonomy.Seeds != 1 || c.Taxonomy.Excluded != 1 || c.Taxonomy.Served != 2 {
		t.Fatalf("method=%s exclude=%v stats=%+v: %v", method, excluded, c.Taxonomy, err)
	}
}

func TestBuild_NoTaxonomyLeavesTablesEmpty(t *testing.T) {
	f := fixture(t)
	c := loadTaxonomyFixture(t, f, TaxonomyInput{})
	if c.Taxonomy != (TaxonomyStats{}) {
		t.Fatalf("stats=%+v", c.Taxonomy)
	}
}

func taxonomyDocument(t *testing.T, f *loadFixture) {
	t.Helper()
	key := "vn:vbpl:1"
	fetch := f.fetch(t, key)
	parent := "article"
	nodes := []chunk.Node{
		{NodeID: "article", Kind: "dieu", Label: "Điều 1", Ordinal: 1, Path: "dieu-1", Text: "Synthetic article"},
		{NodeID: "clause", ParentNodeID: &parent, Kind: "khoan", Label: "1", Ordinal: 1, Path: "dieu-1-khoan-1", Text: "Synthetic clause"},
		{NodeID: "suffix", Kind: "dieu", Label: "Điều 5a", Ordinal: 2, Path: "dieu-5a", Text: "Synthetic article"},
	}
	f.addArtifact(t, "extract", key, map[string]any{"provisions.json": nodes, "report.json": []any{}}, fetch.SHA256)
}

func TestSeeds_ExactItemIDAndArticle(t *testing.T) {
	f := fixture(t)
	taxonomyDocument(t, f)
	f.fetch(t, "vn:vbpl:550e8400-e29b-41d4-a716-446655440001")
	input := taxonomyInputFixture()
	input.seeds = []taxonomySeed{{"1", "1", "1.1.c1"}, {"1", "1", "1.1.c1"}, {"1", "5", "1.1.c1"}, {"other", "1", "1.1.c1"}}
	c := loadTaxonomyFixture(t, f, input)
	var n int
	var family string
	var confidence float64
	err := f.corpus.QueryRow(context.Background(), `SELECT count(*),min(f.code),min(a.confidence) FROM taxonomy_assignment a JOIN taxonomy_node f ON f.id=a.family_id JOIN provision p ON p.id=a.provision_id WHERE p.anchor='dieu-1'`).Scan(&n, &family, &confidence)
	if err != nil || n != 1 || family != "1.1" || confidence != 1 || c.Taxonomy.Seeds != 1 || c.Taxonomy.MissingArticles != 1 {
		t.Fatalf("rows %d family %s confidence %v stats %+v: %v", n, family, confidence, c.Taxonomy, err)
	}
}

func TestEffective_Ancestry(t *testing.T) {
	f := fixture(t)
	taxonomyDocument(t, f)
	input := taxonomyInputFixture()
	input.seeds = []taxonomySeed{{"1", "1", "1.1.c1"}}
	input.overrides = []taxonomy.Override{{DocumentKey: "vn:vbpl:1", Anchor: "dieu-1-khoan-1", Code: "1.1.x1", Exclude: true}}
	c := loadTaxonomyFixture(t, f, input)
	var provisions, documents int
	err := f.corpus.QueryRow(context.Background(), `SELECT provision_count,document_count FROM taxonomy_node WHERE code='1.1.c1'`).Scan(&provisions, &documents)
	if err != nil || provisions != 1 || documents != 1 || c.Taxonomy.Seeds != 1 || c.Taxonomy.Excluded != 1 {
		t.Fatalf("counts %d %d stats %+v: %v", provisions, documents, c.Taxonomy, err)
	}
}

func TestCounts_SubtreeEffective(t *testing.T) {
	f := fixture(t)
	taxonomyDocument(t, f)
	input := taxonomyInputFixture()
	input.seeds = []taxonomySeed{{"1", "1", "1.1.c1"}}
	loadTaxonomyFixture(t, f, input)
	var provisions, documents int
	err := f.corpus.QueryRow(context.Background(), `SELECT provision_count,document_count FROM taxonomy_node WHERE code='1.1'`).Scan(&provisions, &documents)
	if err != nil || provisions != 2 || documents != 1 {
		t.Fatalf("counts %d %d: %v", provisions, documents, err)
	}
}

func TestImportThenBuild_RemovalAndReplacement(t *testing.T) {
	p := taxonomy.Package{ChuDe: []taxonomy.ChuDe{{GUID: "s", STT: 1, Name: "Subject"}}, DeMuc: []taxonomy.DeMuc{{GUID: "old", ChuDeGUID: "s", STT: 1, Name: "Old"}}}
	original, err := taxonomy.Import(p, taxonomy.Mapping{})
	if err != nil {
		t.Fatal(err)
	}
	p.DeMuc = []taxonomy.DeMuc{{GUID: "new", ChuDeGUID: "s", STT: 1, Name: "New"}}
	next, err := taxonomy.Import(p, original)
	if err != nil {
		t.Fatal(err)
	}
	f := fixture(t)
	loadTaxonomyFixture(t, f, TaxonomyInput{version: "fixture", mapping: next})
	var status, name string
	var count int
	err = f.corpus.QueryRow(context.Background(), `SELECT status,name,provision_count FROM taxonomy_node WHERE code='1.1'`).Scan(&status, &name, &count)
	if err != nil || status != "retired" || name != "Old" || count != 0 {
		t.Fatalf("%s %s %d: %v", status, name, count, err)
	}
	var replacements int
	if err = f.corpus.QueryRow(context.Background(), `SELECT count(*) FROM taxonomy_node WHERE name='New' AND code<>'1.1' AND status='active'`).Scan(&replacements); err != nil || replacements != 1 {
		t.Fatalf("replacements=%d: %v", replacements, err)
	}
}

func TestValidateTaxonomy_PopulatedAndTampering(t *testing.T) {
	ctx := context.Background()
	f := fixture(t)
	taxonomyDocument(t, f)
	input := taxonomyInputFixture()
	input.seeds = []taxonomySeed{{"1", "1", "1.1.c1"}, {"1", "5", "1.1.c1"}}
	input.overrides = []taxonomy.Override{
		{DocumentKey: "vn:vbpl:1", Anchor: "dieu-5a", Code: "1.1.x1"},
		{DocumentKey: "vn:vbpl:1", Anchor: "dieu-1-khoan-1", Code: "1.1.x1", Exclude: true},
		{DocumentKey: "vn:vbpl:missing", Anchor: "dieu-1", Code: "1.1.c1"},
	}
	counts := loadTaxonomyFixture(t, f, input)
	stats := counts.Taxonomy
	if stats.Nodes == 0 || stats.Seeds != 1 || stats.Overrides != 1 || stats.Excluded != 1 || stats.MissingArticles != 1 || stats.MissingOverrideTargets != 1 {
		t.Fatalf("stats=%+v", stats)
	}
	record := BuildRecord{TaxonomyVersion: input.version, Taxonomy: &stats}
	if err := validateTaxonomy(ctx, f.corpus, record); err != nil {
		t.Fatalf("valid taxonomy: %v", err)
	}
	for _, tc := range []struct{ name, query string }{
		{"count", `UPDATE taxonomy_node SET provision_count=provision_count+1 WHERE code='1.1.c1'`},
		{"family", `UPDATE taxonomy_assignment SET family_id=(SELECT id FROM taxonomy_node WHERE code='1') WHERE method='phapdien_seed'`},
		{"version", `UPDATE taxonomy_assignment SET taxonomy_version='tampered' WHERE method='phapdien_seed'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := f.corpus.Exec(ctx, "BEGIN"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := f.corpus.Exec(ctx, "ROLLBACK"); err != nil {
					t.Error(err)
				}
			})
			tag, err := f.corpus.Exec(ctx, tc.query)
			if err != nil || tag.RowsAffected() != 1 {
				t.Fatalf("tamper rows=%d: %v", tag.RowsAffected(), err)
			}
			if err = validateTaxonomy(ctx, f.corpus, record); !errors.Is(err, errInvalidDatabase) {
				t.Fatalf("tampering accepted or unexpected error: %v", err)
			}
		})
	}
}
