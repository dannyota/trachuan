//go:build integration

package retrieval

import (
	"context"
	"testing"
)

func TestNotYetEligible_PrimaryWithCurrentFalse(t *testing.T) {
	db, engine, _ := fixture(t)
	ctx := context.Background()
	if _, err := db.Exec(ctx, `UPDATE document SET status_class='not_yet', is_current=false, eff_from='2099-01-01' WHERE id=2`); err != nil {
		t.Fatal(err)
	}
	engine.options.Profile = "evaluation"
	prepared, err := engine.Prepare(ctx, Request{Query: "ngân hàng", Mode: "exact"})
	if err != nil {
		t.Fatal(err)
	}
	recipe := engine.release.RetrievalRecipe
	arms, err := engine.CollectWithNotYetEligible(ctx, prepared, Depths{Dense: recipe.DenseDepth, Lexical: recipe.LexicalDepth})
	if err != nil {
		t.Fatal(err)
	}
	response, err := Shape(arms, recipe, 8, "dense")
	if err != nil {
		t.Fatal(err)
	}
	for _, hit := range response.Hits {
		if hit.Stamp.StatusClass == "not_yet" {
			if hit.Stamp.Current {
				t.Fatal("not-yet hit marked current")
			}
			return
		}
	}
	t.Fatal("not-yet document missing from primary hits")
}
