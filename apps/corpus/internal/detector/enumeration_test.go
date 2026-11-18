package detector

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/detector/gapsource"
)

func TestRegistry_MatchesGapKindsInCode(t *testing.T) {
	files, err := gapsource.Parse("..")
	if err != nil {
		t.Fatal(err)
	}
	sources := gapsource.Sources(files)
	if len(sources) < 80 {
		t.Fatalf("gap source enumeration incomplete: %d kinds", len(sources))
	}
	for kind, dirs := range sources {
		if _, ok := Lookup(kind); !ok {
			t.Errorf("gap kind %q written in %v is not registered", kind, dirs)
		}
	}
	for _, d := range All() {
		if _, ok := sources[d.ID]; !ok {
			t.Errorf("registered gap kind %q is never written in code", d.ID)
		}
	}
}
