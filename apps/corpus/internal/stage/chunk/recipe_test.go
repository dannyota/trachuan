package chunk

import (
	"encoding/json"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/stage"
)

func TestRecipe_ListingOnlyIdentity(t *testing.T) {
	const want = `{"version":2,"rough_count":"max(1,runes/2)","body_limit":512,"minimum_runes":20,"traversal":"parent-node-id-depth-first","parent_lead":"substantive-before-first-child","sharding":"lines-then-words","dedup":"nfc-collapsed-whitespace","outside_articles":"own-text","prefix_fields":"number-title-chapter-section-date","prefix_field_limit":220,"prefix_number_limit":128,"date_normalizer":"corpus-normalizer/3","listing_only_doc_types":["Văn bản hệ thống hóa"]}`
	recipe := Recipe2()
	data, err := json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Fatalf("recipe=%s, want %s", data, want)
	}
	if recipe.Hash() != stage.RecipeHash("chunk", "vn-article/2", want) {
		t.Fatalf("wrong recipe identity: %s", recipe.Hash())
	}
	if recipe.Hash() == "620e0ae95a786f399500349dec089c779fba9900f84d52e6000a34152a176cb7" {
		t.Fatal("listing-only rules did not change the recipe hash")
	}
}
