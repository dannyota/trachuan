package fixture

import (
	"os"
	"strings"
	"testing"
)

func TestE2EScript_FlagOffReleaseInsert(t *testing.T) {
	b, err := os.ReadFile("../../../../../../scripts/e2e.sh")
	if err != nil {
		t.Fatal(err)
	}
	want := `else
  printf "INSERT INTO corpus_release VALUES ('vn', :'db', now(), 'e2e')\n" | psql -X "$auth_url" -v ON_ERROR_STOP=1 -v db="$fixture_name"
fi`
	if !strings.Contains(string(b), want) {
		t.Fatal("flag-off release insert changed or missing")
	}
}
