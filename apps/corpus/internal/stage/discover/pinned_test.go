package discover

import (
	"context"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage"
	"danny.vn/trachuan/apps/corpus/internal/stage/membership"
	"github.com/jackc/pgx/v5"
)

func TestStage_PinPreflight(t *testing.T) {
	if _, ok := any(New(nil)).(interface {
		Preflight(context.Context, stage.Env, pgx.Tx, stage.Run) (stage.Finalization, error)
	}); !ok {
		t.Fatal("discovery can list before validating pins")
	}
}

func TestPinned_SeedsAndConflicts(t *testing.T) {
	pin := source.Pin{Key: "vn:vbpl:p", Number: "71/2099/QH15", Expand: "expand", Fetch: "fetch"}
	for _, test := range []struct {
		name, key, source string
		conflict          bool
	}{
		{"absent", "", "", false}, {"VBPL duplicate", "vn:vbpl:other", "vbpl", true}, {"secondary candidate", "vn:chinhphu:2", "chinhphu", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			members := map[string]*seedMember{}
			number := "71/2099/QH15"
			if test.key != "" {
				members[test.key] = &seedMember{entry: membership.Entry{DocumentKey: test.key, Source: test.source, Disposition: "active", NormalizedNumber: &number}}
			}
			got := addPinnedMembers(stage.Run{Country: "vn", Snapshot: stage.Snapshot{SourceListHash: "sources"}}, []source.Pin{pin}, members)
			if (got.Code == "pin_conflict") != test.conflict {
				t.Fatalf("failure=%+v", got)
			}
			if test.conflict {
				return
			}
			m := members[pin.Key]
			if m == nil || m.entry.Class != "primary" || m.entry.Disposition != "active" || len(m.records) != 1 || m.records[0].Support.RuleKind != "pinned" || !directSupport(m.records[0].Support) {
				t.Fatalf("pin seed=%+v", m)
			}
		})
	}
}

func TestPinned_ListedKeyKeepsFrozenNumber(t *testing.T) {
	pin := source.Pin{Key: "vn:vbpl:p", Number: "71/2099/QH15"}
	changed := "72/2099/QH15"
	members := map[string]*seedMember{pin.Key: {entry: membership.Entry{DocumentKey: pin.Key, Source: "vbpl", SourceID: "p", Disposition: "active", RawNumber: &changed, NormalizedNumber: &changed}}}
	failure := addPinnedMembers(stage.Run{Country: "vn"}, []source.Pin{pin}, members)
	m := members[pin.Key]
	if failure.Code != "" || m.entry.RawNumber == nil || *m.entry.RawNumber != pin.Number || m.entry.NormalizedNumber == nil || *m.entry.NormalizedNumber != pin.Number {
		t.Fatalf("pin=%+v failure=%+v", m.entry, failure)
	}
}

func TestPinned_ConflictsWithAnyCurrentListingNumber(t *testing.T) {
	pin := source.Pin{Key: "vn:vbpl:p", Number: "71/2099/QH15"}
	otherNumber := "72/2099/QH15"
	members := map[string]*seedMember{"vn:vbpl:other": {entry: membership.Entry{DocumentKey: "vn:vbpl:other", Source: "vbpl", Disposition: "active", NormalizedNumber: &otherNumber}, numbers: map[string]bool{pin.Number: true, otherNumber: true}}}
	if failure := addPinnedMembers(stage.Run{Country: "vn"}, []source.Pin{pin}, members); failure.Code != "pin_conflict" {
		t.Fatalf("failure=%+v", failure)
	}
}
