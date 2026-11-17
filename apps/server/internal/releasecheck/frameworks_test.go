package releasecheck

import (
	"encoding/json"
	"net/http"
	"testing"

	"danny.vn/trachuan/internal/embedding"
)

func TestDescribeFrameworks_ClosedShapeAndRunningQueryHash(t *testing.T) {
	for _, recipe := range []embedding.Recipe{embedding.QueryRecipe1(), embedding.TestRecipe()} {
		client := embedding.NewClient("http://embedder", &http.Client{Transport: recipeTransport{embedding.Report{ProtocolVersion: 1, RecipeHash: recipe.Hash(), Recipe: recipe}}})
		report, err := DescribeFrameworks(t.Context(), client)
		if err != nil {
			t.Fatal(err)
		}
		raw, err := json.Marshal(report)
		if err != nil {
			t.Fatal(err)
		}
		want := `{"schema_versions":[1],"retrieval_recipe_versions":[2],"lexical_recipe_versions":[1],"query_recipe_hash":"` + recipe.Hash() + `"}`
		if string(raw) != want {
			t.Fatalf("unexpected report: %s", raw)
		}
	}
}

func TestFrameworksDatabaseConfig_RequiresReaderReadOnlyAndLogGuard(t *testing.T) {
	for _, raw := range []string{"postgres://postgres@localhost/{database}", "postgres://corpus_reader@localhost/{database}?user=postgres", "invalid"} {
		if _, err := frameworksDatabaseConfig(raw, "frameworks_20990101"); err == nil {
			t.Fatal("accepted unsafe reader")
		}
	}
	for _, name := range []string{"corpus_vn_20990101", "frameworks_20990101_bad", "auth"} {
		if _, err := frameworksDatabaseConfig("postgres://corpus_reader@localhost/{database}", name); err == nil {
			t.Fatal("accepted unsafe name")
		}
	}
	cfg, err := frameworksDatabaseConfig("postgres://corpus_reader@localhost/{database}?default_transaction_read_only=off", "frameworks_20990101_1")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ConnConfig.RuntimeParams["default_transaction_read_only"] != "on" || cfg.AfterConnect == nil {
		t.Fatal("missing safety guard")
	}
}

func TestFrameworksReleaseIdentity_RequiresSupportedSchemaAndMatchingName(t *testing.T) {
	for _, tc := range []struct {
		name, rel string
		version   int
		valid     bool
	}{
		{"frameworks_20990101", "20990101", 1, true},
		{"frameworks_20990101_1", "20990101_1", 1, true},
		{"frameworks_20990101_99", "20990101_99", 1, true},
		{"frameworks_20990101_123", "20990101_123", 1, false},
		{"frameworks_20990101", "20990102", 1, false},
		{"frameworks_20990101", "20990101", 2, false},
		{"corpus_vn_20990101", "20990101", 1, false},
	} {
		if validFrameworksIdentity(tc.name, tc.rel, tc.version) != tc.valid {
			t.Fatalf("identity accepted incorrectly: %+v", tc)
		}
	}
}

func TestFrameworksDatabaseConfig_RejectsUnsupportedSuffix(t *testing.T) {
	for _, name := range []string{"frameworks_20990101_", "frameworks_20990101_100", "frameworks_20990101_1234"} {
		if _, err := frameworksDatabaseConfig("postgres://corpus_reader@localhost/{database}", name); err == nil {
			t.Fatalf("configured unsupported suffix %s", name)
		}
	}
}
