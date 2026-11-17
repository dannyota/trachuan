package queryfile

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDistinct_PreservesSuiteFirst(t *testing.T) {
	got := distinct([]string{" one   two ", "heading", "one two", "", "title\nname", "heading"})
	if !reflect.DeepEqual(got, []string{"one two", "heading", "title name"}) {
		t.Fatalf("got %v", got)
	}
}

func TestSuiteQuestions_LegacyBeforeDevelopment(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"legacy.json": `[{"question":"legacy"}]`, "development.json": `[{"question":"development"}]`} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	values, err := suiteQuestions(dir)
	if err != nil || !reflect.DeepEqual(values, []string{"legacy", "development"}) {
		t.Fatalf("%v %v", values, err)
	}
}
