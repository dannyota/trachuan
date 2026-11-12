package extract

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

type frozenDB struct {
	rows map[store.GetRunOCRSelectionParams]store.GetRunOCRSelectionRow
}

func (f *frozenDB) GetRunOCRSelection(_ context.Context, key store.GetRunOCRSelectionParams) (store.GetRunOCRSelectionRow, error) {
	if row, ok := f.rows[key]; ok {
		return row, nil
	}
	return store.GetRunOCRSelectionRow{}, pgx.ErrNoRows
}
func (f *frozenDB) InsertRunOCRSelection(_ context.Context, p store.InsertRunOCRSelectionParams) error {
	key := store.GetRunOCRSelectionParams{RunID: p.RunID, ItemKey: p.ItemKey}
	if _, ok := f.rows[key]; !ok {
		f.rows[key] = store.GetRunOCRSelectionRow{RunID: p.RunID, ItemKey: p.ItemKey, Gap: p.Gap}
	}
	return nil
}
func TestFrozenSelection_StoreChangesAndCrashReplay(t *testing.T) {
	q := &frozenDB{rows: map[store.GetRunOCRSelectionParams]store.GetRunOCRSelectionRow{}}
	current := "text_source_ambiguous"
	calls := 0
	choose := func() (selection, error) {
		calls++
		return selection{source: "none", gap: current, consulted: true}, nil
	}
	first, err := planFrozenSelection(context.Background(), q, 1, "doc", choose)
	if err != nil || first.gap != current {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	current = "needs_ocr"
	resumed, err := planFrozenSelection(context.Background(), q, 1, "doc", choose)
	if err != nil || resumed.gap != first.gap || calls != 1 || selectionInputHash("fetch", resumed) != selectionInputHash("fetch", first) {
		t.Fatalf("replan=%+v calls=%d err=%v", resumed, calls, err)
	}
	next, err := planFrozenSelection(context.Background(), q, 2, "doc", choose)
	if err != nil || next.gap != current || selectionInputHash("fetch", next) == selectionInputHash("fetch", first) {
		t.Fatalf("new run=%+v err=%v", next, err)
	}
}

func TestFrozenSelection_LabelFlagged(t *testing.T) {
	q := &frozenDB{rows: map[store.GetRunOCRSelectionParams]store.GetRunOCRSelectionRow{}}
	choose := func() (selection, error) {
		return selection{source: "none", gap: "ocr_label_flagged", reason: "checker flagged an article label or document number", consulted: true}, nil
	}
	got, err := planFrozenSelection(context.Background(), q, 1, "doc", choose)
	if err != nil || got.source != "none" || got.gap != "ocr_label_flagged" || got.reason != "checker flagged an article label or document number" || !got.consulted {
		t.Fatalf("selection=%+v err=%v", got, err)
	}
}

func TestFrozenSelection_NumberMismatch(t *testing.T) {
	q := &frozenDB{rows: map[store.GetRunOCRSelectionParams]store.GetRunOCRSelectionRow{}}
	got, err := planFrozenSelection(context.Background(), q, 1, "doc", func() (selection, error) {
		return selection{source: "none", gap: "text_number_mismatch", consulted: true}, nil
	})
	if err != nil || got.gap != "text_number_mismatch" || !got.consulted {
		t.Fatalf("selection=%+v err=%v", got, err)
	}
}
