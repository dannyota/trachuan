//go:build integration

package retrieval

import (
	"context"
	"reflect"
	"testing"
)

func TestCollectWithLexical_MatchesFreshHydratedArms(t *testing.T) {
	_, engine, _ := fixture(t)
	ctx := context.Background()
	for _, query := range []string{"ngân hàng", "tín dụng", "hết hạn"} {
		t.Run(query, func(t *testing.T) {
			prepared, err := engine.Prepare(ctx, Request{Query: query, Mode: "exact"})
			if err != nil {
				t.Fatal(err)
			}
			exact, err := engine.Collect(ctx, prepared, Depths{Dense: 200, Lexical: 100})
			if err != nil {
				t.Fatal(err)
			}
			prepared.Request.Mode = "ann"
			fresh, err := engine.Collect(ctx, prepared, Depths{Dense: 50, Lexical: 50})
			if err != nil {
				t.Fatal(err)
			}
			reused, err := engine.CollectWithLexical(ctx, prepared, Depths{Dense: 50, Lexical: 50}, LexicalArms{Primary: exact.Primary.Lexical, NonCurrent: exact.NonCurrent.Lexical, Depth: exact.Depths.Lexical})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(fresh.Primary, reused.Primary) || !reflect.DeepEqual(fresh.NonCurrent, reused.NonCurrent) {
				t.Fatal("reused lexical arms differ from fresh arms")
			}
		})
	}
}
