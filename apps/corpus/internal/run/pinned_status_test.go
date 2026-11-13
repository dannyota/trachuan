package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/pinned"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5"
)

func TestStatus_WithdrawnNamedFirst(t *testing.T) {
	prior := []membership.Entry{{DocumentKey: "new", Disposition: "active"}, {DocumentKey: "old", Disposition: "withdrawn"}}
	current := []membership.Entry{
		{DocumentKey: "old", Disposition: "withdrawn", Supports: []membership.SupportTuple{{RuleKind: "named"}}},
		{DocumentKey: "new", Disposition: "withdrawn", RawNumber: textPtr("71/2099/QH15"), Supports: []membership.SupportTuple{{RuleKind: "named"}}},
		{DocumentKey: "ordinary", Disposition: "withdrawn", Supports: []membership.SupportTuple{{RuleKind: "agency"}}},
	}
	docs := newlyWithdrawnNamed(current, prior)
	if len(docs) != 1 || docs[0].Key != "new" || docs[0].Number != "71/2099/QH15" {
		t.Fatalf("withdrawn=%+v", docs)
	}
	r := Report{NewlyWithdrawnNamed: docs, PinNotices: []pinned.Notice{{Kind: "source_pin_live", Key: "live"}}}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Index(string(data), `"newly_withdrawn_named"`) > strings.Index(string(data), `"source_pin_live"`) {
		t.Fatalf("status order=%s", data)
	}
}
func textPtr(value string) *string { return &value }

type statusAcquisitionDB struct {
	store.DBTX
	rows []store.ListDocumentAcquisitionsRow
}

func (db statusAcquisitionDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return &statusAcquisitionRows{rows: db.rows}, nil
}

type statusAcquisitionRows struct {
	pgx.Rows
	rows  []store.ListDocumentAcquisitionsRow
	index int
}

func (rows *statusAcquisitionRows) Close()     {}
func (rows *statusAcquisitionRows) Err() error { return nil }
func (rows *statusAcquisitionRows) Next() bool { rows.index++; return rows.index <= len(rows.rows) }
func (rows *statusAcquisitionRows) Scan(dest ...any) error {
	row := rows.rows[rows.index-1]
	for i, value := range []string{row.Rel, row.Stage, row.Path, row.Sha256} {
		*dest[i].(*string) = value
	}
	return nil
}

func TestStatus_UsesNewestVerifiedPairFromOneRelease(t *testing.T) {
	for _, name := range []string{"split latest stages", "newest complete pair", "corrupt newest fetch", "no verified pair", "no acquisitions"} {
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			artifacts := artifact.Store{Root: t.TempDir()}
			var rows []store.ListDocumentAcquisitionsRow
			add := func(rel, stageName string) store.ListDocumentAcquisitionsRow {
				t.Helper()
				art, err := artifacts.PublishContent(ctx, stageName, stage.RecipeHash(stageName, rel), func(dir string) error {
					data := []byte(fmt.Sprintf(`{"data":{"id":"pin","docNum":"71/2099/QH15"},"fixture":"%s/%s"}`, rel, stageName))
					return os.WriteFile(filepath.Join(dir, "detail.json"), data, 0600)
				})
				if err != nil {
					t.Fatal(err)
				}
				row := store.ListDocumentAcquisitionsRow{Rel: rel, Stage: stageName, Path: art.Path, Sha256: art.SHA256}
				rows = append(rows, row)
				return row
			}
			expectedRel := "20990101_9"
			expected := map[string]string{}
			if name != "no verified pair" && name != "no acquisitions" {
				for _, stageName := range []string{"expand", "fetch"} {
					row := add(expectedRel, stageName)
					expected[stageName] = row.Sha256
				}
			} else {
				expectedRel = ""
			}
			switch name {
			case "split latest stages", "no verified pair":
				add("20990101_11", "expand")
				add("20990101_10", "fetch")
			case "newest complete pair":
				expectedRel = "20990101_11"
				for _, stageName := range []string{"expand", "fetch"} {
					row := add(expectedRel, stageName)
					expected[stageName] = row.Sha256
				}
			case "corrupt newest fetch":
				add("20990101_11", "expand")
				bad := add("20990101_11", "fetch")
				if err := os.WriteFile(filepath.Join(artifacts.Abs(bad.Path), "detail.json"), []byte("altered"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			report := Report{Run: store.Run{Country: "vn", Rel: "20990102_1"}}
			doc := WithdrawnNamed{Key: "vn:vbpl:pin", Number: "71/2099/QH15"}
			if err := report.loadNamedAcquisitions(ctx, store.New(statusAcquisitionDB{rows: rows}), artifacts, &doc); err != nil {
				t.Fatal(err)
			}
			data, err := json.Marshal(doc)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err = json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if expectedRel == "" {
				if string(fields["message"]) != `"no verified pair"` || fields["rel"] != nil || fields["expand"] != nil || fields["fetch"] != nil {
					t.Fatalf("expected no verified pair: %s", data)
				}
			} else {
				if string(fields["rel"]) != fmt.Sprintf("%q", expectedRel) {
					t.Fatalf("expected pair from %s: %s", expectedRel, data)
				}
				var pin source.Pin
				if err = json.Unmarshal(data, &pin); err != nil {
					t.Fatal(err)
				}
				if pin.Key != doc.Key || pin.Number != doc.Number || pin.Expand != expected["expand"] || pin.Fetch != expected["fetch"] || fields["message"] != nil {
					t.Fatalf("status cannot form a pin: %s", data)
				}
			}
			if doc.Key != "vn:vbpl:pin" || doc.Number != "71/2099/QH15" {
				t.Fatalf("lost key or number: %s", data)
			}
		})
	}
}
