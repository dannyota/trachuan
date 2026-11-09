package retrieval

import (
	"testing"

	"danny.vn/trachuan/internal/embedding"
)

func TestValidateDenseRecipe_RejectsMismatch(t *testing.T) {
	query := embedding.QueryRecipe1()
	doc := embedding.DocumentRecipe1()
	dense := DenseRecipe{Version: 1, Model: query.Model, Dimensions: query.Dimensions, Document: doc, DocumentHash: doc.Hash(), Query: query, QueryHash: query.Hash()}
	if err := validateDenseRecipe(dense, query, query.Dimensions); err != nil {
		t.Fatal(err)
	}
	dense.QueryHash = doc.Hash()
	if err := validateDenseRecipe(dense, query, query.Dimensions); err == nil {
		t.Fatal("accepted a mismatched query hash")
	}
	dense.QueryHash = query.Hash()
	if err := validateDenseRecipe(dense, query, query.Dimensions-1); err == nil {
		t.Fatal("accepted a mismatched typmod")
	}
}
