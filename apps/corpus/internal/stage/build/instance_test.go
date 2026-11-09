package build

import (
	"strings"
	"testing"
)

func TestOwnershipMarker_IdentityAndInstance(t *testing.T) {
	input, recipe := strings.Repeat("a", 64), strings.Repeat("b", 64)
	built := ownershipMarker{RunKey: "vn/20261109", InputHash: input, RecipeHash: recipe, Instance: "built"}
	encoded, err := encodeOwnershipMarker(built)
	if err != nil {
		t.Fatal(err)
	}
	restored := built
	restored.Instance = "restored"
	other, err := encodeOwnershipMarker(restored)
	if err != nil {
		t.Fatal(err)
	}
	if !sameOwnership(encoded, other) {
		t.Fatal("instance changed ownership identity")
	}
	got, err := parseOwnershipMarker(other)
	if err != nil || got != restored {
		t.Fatalf("restored marker: %+v, %v", got, err)
	}
	restored.RecipeHash = strings.Repeat("c", 64)
	foreign, err := encodeOwnershipMarker(restored)
	if err != nil {
		t.Fatal(err)
	}
	if sameOwnership(encoded, foreign) {
		t.Fatal("accepted foreign recipe")
	}
	legacy := "vn/20261109:" + input + ":" + recipe
	if !sameOwnership(encoded, legacy) {
		t.Fatal("legacy build marker not recognized")
	}
	if _, err = parseOwnershipMarker(`{"instance":"unknown"}`); err == nil {
		t.Fatal("accepted malformed marker")
	}
}
