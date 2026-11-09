package build

import (
	"strings"
	"testing"
)

func TestRestore_FiltersExtensionsAndRejectsFunctions(t *testing.T) {
	list := "; archive\n1; 3079 1 EXTENSION - vector owner\n2; 3079 2 EXTENSION - ltree owner\n3; 1259 3 TABLE public chunk owner\n"
	got, err := filterRestoreList(list)
	if err != nil || strings.Contains(got, "EXTENSION") || !strings.Contains(got, "TABLE public chunk") {
		t.Fatalf("filtered list %q: %v", got, err)
	}
	for _, bad := range []string{"4; 3079 4 EXTENSION - unsafe owner", "5; 1255 5 FUNCTION public evil() owner", "6; 3466 6 EVENT TRIGGER - evil owner"} {
		if _, err = filterRestoreList(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}
