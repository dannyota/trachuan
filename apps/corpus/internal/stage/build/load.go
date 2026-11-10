package build

import (
	"context"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/internal/embedding"
	"danny.vn/trachuan/internal/retrieval/lexical"
	"github.com/jackc/pgx/v5"
)

// Recipes identifies the dense and lexical encoders used for the build.
type Recipes struct {
	Dense   embedding.Recipe
	Lexical lexical.Recipe
}

// Counts reports rows and lexical statistics loaded into the corpus.
type Counts struct {
	Documents, SourceFiles, Provisions, Chunks, DenseVectors, Rejected, UnavailableArtifacts, Relations, Gaps int64
	LexicalStats                                                                                              lexical.StatsSummary
	validity                                                                                                  validityResult
}

type loader struct {
	tx              pgx.Tx
	inv             Inventory
	artifacts       artifact.Store
	recipes         Recipes
	codes           map[string]source.StatusCode
	items           map[string]map[string]InventoryItem
	docs            map[string]*loadedDocument
	present         map[string]bool
	members         map[string]membership.Entry
	provenanceFiles map[string][]provenanceFile
	gaps            []InventoryGap
	counts          Counts
}

// Load inserts a frozen inventory into an already initialized corpus database.
func Load(ctx context.Context, conn *pgx.Conn, inv Inventory, artifacts artifact.Store, recipes Recipes, codes map[string]source.StatusCode, corrections []source.Correction, asOf string) (Counts, error) {
	if conn == nil {
		return Counts{}, fmt.Errorf("corpus connection is nil")
	}
	if recipes.Lexical != lexical.Recipe1() {
		return Counts{}, fmt.Errorf("unsupported lexical recipe")
	}
	for code, v := range codes {
		current := v.Class == "in_force" || v.Class == "partial"
		valid := current || v.Class == "expired" || v.Class == "not_yet" || v.Class == "suspended" || v.Class == "unknown"
		if !valid || current != v.Current {
			return Counts{}, fmt.Errorf("invalid status mapping %q", code)
		}
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return Counts{}, fmt.Errorf("begin corpus load: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	l := loader{tx: tx, inv: inv.sorted(), artifacts: artifacts, recipes: recipes, codes: codes, items: make(map[string]map[string]InventoryItem), docs: make(map[string]*loadedDocument), present: inv.Presence(), members: map[string]membership.Entry{}, provenanceFiles: map[string][]provenanceFile{}, gaps: append([]InventoryGap{}, inv.Gaps...)}
	for _, item := range l.inv.Items {
		if l.items[item.Stage] == nil {
			l.items[item.Stage] = make(map[string]InventoryItem)
		}
		l.items[item.Stage][item.ItemKey] = item
	}
	for _, file := range inv.Provenance.Files {
		l.provenanceFiles[file.DocumentKey] = append(l.provenanceFiles[file.DocumentKey], file)
	}
	if l.inv.Version == 2 {
		for _, member := range l.inv.Membership.Entries {
			l.members[member.DocumentKey] = member
			if member.Disposition != "active" {
				l.addGap("build", member.DocumentKey, member.Disposition, "")
			}
		}
	}
	if err = l.reconcileInventory(); err != nil {
		return Counts{}, err
	}
	if err = l.loadDocuments(ctx); err != nil {
		return Counts{}, err
	}
	if _, err = tx.Exec(ctx, `CREATE TEMP TABLE build_chunk ON COMMIT DROP AS SELECT id,document_id,provision_id,ordinal,context_prefix,content,spans,input_sha256,token_count,embedding FROM chunk WITH NO DATA; CREATE UNIQUE INDEX ON build_chunk(id)`); err != nil {
		return Counts{}, fmt.Errorf("create chunk staging table: %w", err)
	}
	if err = l.loadChunks(ctx); err != nil {
		return Counts{}, err
	}
	l.counts.LexicalStats, err = loadLexical(ctx, tx, recipes.Lexical)
	if err != nil {
		return Counts{}, err
	}
	if err = l.loadRelations(ctx); err != nil {
		return Counts{}, err
	}
	if err = l.deriveConsolidated(ctx); err != nil {
		return Counts{}, err
	}
	l.counts.validity, err = l.loadValidity(ctx, corrections, asOf)
	if err != nil {
		return Counts{}, err
	}
	if err = l.loadGaps(ctx); err != nil {
		return Counts{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Counts{}, fmt.Errorf("commit corpus load: %w", err)
	}
	return l.counts, nil
}
