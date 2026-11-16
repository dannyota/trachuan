package classify

import "testing"

func TestValidateArticles_DropsAndMergesCodes(t *testing.T) {
	req := ArticleRequest{Articles: articleInputs(1, ""), Nodes: articleNodes()[1:4]}
	out, _ := ValidateArticles("end_turn", `{"dieu-1":{"codes":[{"code":"1.1.c1","confidence":0.6},{"code":"1.1.c1","confidence":0.9},{"code":"1.1.x1","confidence":0.8},{"code":"1.1.old","confidence":1},{"code":"2.1","confidence":1}]}}`, req, articleNodes())
	v := out["dieu-1"]
	if v.Outcome != "ok" || len(v.Codes) != 2 || v.Codes[0].Confidence != 0.9 || len(v.Dropped) != 2 {
		t.Fatalf("outcome %+v", v)
	}
}

func TestValidateArticles_RejectsSchemaAndStopFailures(t *testing.T) {
	req := ArticleRequest{Articles: articleInputs(2, ""), Nodes: articleNodes()[1:4]}
	valid := `{"dieu-1":{"codes":[]},"dieu-2":{"codes":[]}}`
	for _, tc := range []struct{ stop, text string }{{"max_tokens", valid}, {"refusal", valid}, {"end_turn", `{"dieu-1":{"codes":null},"dieu-2":{"codes":[]}}`}, {"end_turn", `{"dieu-1":{"codes":[]},"dieu-1":{"codes":[]},"dieu-2":{"codes":[]}}`}, {"end_turn", `{"dieu-1":{"codes":[{"code":"1.1","confidence":null}]},"dieu-2":{"codes":[]}}`}, {"end_turn", valid + ` {}`}, {"end_turn", valid[:len(valid)-1]}} {
		out, _ := ValidateArticles(tc.stop, tc.text, req, articleNodes())
		for _, v := range out {
			if v.Outcome != "invalid_response" {
				t.Fatalf("accepted %s: %+v", tc.text, out)
			}
		}
	}
	t.Run("foreign anchor", func(t *testing.T) {
		out, dropped := ValidateArticles("end_turn", `{"dieu-1":{"codes":[{"code":"1.1","confidence":0.9}]},"dieu-2":{"codes":[]},"extra":null}`, req, articleNodes())
		if dropped != 1 || len(out) != 2 || out["dieu-1"].Outcome != "ok" || len(out["dieu-1"].Codes) != 1 || out["dieu-2"].Outcome != "ok" {
			t.Fatalf("foreign anchor voided requested results: %+v", out)
		}
	})
	t.Run("missing anchor", func(t *testing.T) {
		out, _ := ValidateArticles("end_turn", `{"dieu-1":{"codes":[]}}`, req, articleNodes())
		if out["dieu-1"].Outcome != "ok" || out["dieu-2"].Outcome != "invalid_response" {
			t.Fatalf("missing anchor voided its peer: %+v", out)
		}
	})
}

func TestValidateArticles_ConfidenceFailureIsPerArticle(t *testing.T) {
	req := ArticleRequest{Articles: articleInputs(2, ""), Nodes: articleNodes()[1:4]}
	for _, confidence := range []string{"-0.1", "1.1", "1e400"} {
		out, _ := ValidateArticles("end_turn", `{"dieu-1":{"codes":[{"code":"1.1","confidence":`+confidence+`}]},"dieu-2":{"codes":[{"code":"1.1","confidence":1}]}}`, req, articleNodes())
		if out["dieu-1"].Outcome != "invalid_response" || out["dieu-2"].Outcome != "ok" {
			t.Fatalf("outcome %+v", out)
		}
	}
}
