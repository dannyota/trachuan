package detector

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestRegistry_DeclaresEveryField(t *testing.T) {
	all := All()
	if len(all) < 80 {
		t.Fatalf("registry has %d detectors", len(all))
	}
	for _, d := range all {
		if d.ID == "" || d.Version < 1 || d.Semantics < 1 || d.Subject == "" || len(d.Inputs) == 0 || len(d.Fingerprint) == 0 || d.Discriminator == "" || d.Completeness == "" {
			t.Errorf("incomplete detector %+v", d)
		}
		if d.Severity != Blocking && d.Severity != Review && d.Severity != Info {
			t.Errorf("detector %s has severity %q", d.ID, d.Severity)
		}
	}
}

func TestRegistry_SortedWithUniqueIDs(t *testing.T) {
	ids := []string{}
	for _, d := range All() {
		ids = append(ids, d.ID)
	}
	if !slices.IsSorted(ids) {
		t.Fatal("detectors are not sorted by id")
	}
	if len(slices.Compact(slices.Clone(ids))) != len(ids) {
		t.Fatal("duplicate detector id")
	}
}

func TestLookup_GapKindDetector(t *testing.T) {
	d, ok := Lookup("source_unavailable")
	if !ok || d.Version != 1 || d.Semantics != 1 || d.Subject != SubjectStageItem || d.Discriminator != DiscriminatorGapDetail || d.Completeness != CompletenessStageItem {
		t.Fatalf("lookup = %+v, %v", d, ok)
	}
	if _, ok := Lookup("future_gap"); ok {
		t.Fatal("unknown id found")
	}
}

func TestLookup_ReturnsCopy(t *testing.T) {
	d, _ := Lookup("source_unavailable")
	d.Fingerprint[0] = "changed"
	again, _ := Lookup("source_unavailable")
	if again.Fingerprint[0] == "changed" {
		t.Fatal("lookup exposed registry storage")
	}
}

func TestValidateKind_RefusesUnregisteredKind(t *testing.T) {
	if err := ValidateKind("source_unavailable"); err != nil {
		t.Fatal(err)
	}
	for _, kind := range []string{"future_gap", ""} {
		err := ValidateKind(kind)
		if !errors.Is(err, ErrUnregisteredGap) || !strings.Contains(err.Error(), `"`+kind+`"`) {
			t.Errorf("ValidateKind(%q) = %v", kind, err)
		}
	}
}

func TestHash_StableAndOrderIndependent(t *testing.T) {
	all := All()
	reversed := slices.Clone(all)
	slices.Reverse(reversed)
	if hashOf(all) != hashOf(reversed) {
		t.Fatal("hash depends on input order")
	}
	if Hash() != hashOf(all) {
		t.Fatal("hash is not stable")
	}
	sum := sha256.Sum256(Canonical())
	if Hash() != hex.EncodeToString(sum[:]) {
		t.Fatal("hash is not the SHA-256 of the canonical form")
	}
}

func TestHash_ChangesWithEveryDetectorField(t *testing.T) {
	base := hashOf(All())
	typ := reflect.TypeOf(Detector{})
	for i := range typ.NumField() {
		detectors := All()
		field := reflect.ValueOf(&detectors[0]).Elem().Field(i)
		switch field.Kind() {
		case reflect.String:
			field.SetString(field.String() + "x")
		case reflect.Int:
			field.SetInt(field.Int() + 1)
		case reflect.Slice:
			field.Set(reflect.Append(field, reflect.ValueOf("zzz")))
		default:
			t.Fatalf("field %s has unhandled kind %s", typ.Field(i).Name, field.Kind())
		}
		if hashOf(detectors) == base {
			t.Errorf("changing %s left the hash unchanged", typ.Field(i).Name)
		}
	}
	if hashOf(All()[1:]) == base {
		t.Error("removing a detector left the hash unchanged")
	}
}

func TestHash_PinsCurrentRegistry(t *testing.T) {
	const want = "f453e7f2ca9bc0a4f0c42ce5e060d6fe1383e2c96aa8b498afe8ad3bae7bd9eb"
	if got := Hash(); got != want {
		t.Fatalf("registry hash = %s, want %s; a registry change must update this pin on purpose", got, want)
	}
}
