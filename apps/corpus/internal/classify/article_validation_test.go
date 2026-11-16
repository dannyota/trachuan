package classify

import "testing"

func TestValidateArticles_DropsAndMergesCodes(t *testing.T) {
	req := ArticleRequest{Articles: articleInputs(1, ""), Nodes: articleNodes()[1:4]}
	out := ValidateArticles("end_turn", `{"dieu-1":{"codes":[{"code":"1.1.c1","confidence":0.6},{"code":"1.1.c1","confidence":0.9},{"code":"1.1.x1","confidence":0.8},{"code":"1.1.old","confidence":1},{"code":"2.1","confidence":1}]}}`, req, articleNodes())
	v := out["dieu-1"]
	if v.Outcome != "ok" || len(v.Codes) != 2 || v.Codes[0].Confidence != 0.9 || len(v.Dropped) != 2 {
		t.Fatalf("outcome %+v", v)
	}
}

func TestValidateArticles_RejectsSchemaAndStopFailures(t *testing.T) {
	req := ArticleRequest{Articles: articleInputs(2, ""), Nodes: articleNodes()[1:4]}
	valid := `{"dieu-1":{"codes":[]},"dieu-2":{"codes":[]}}`
	for _, tc := range []struct{ stop, text string }{{"max_tokens", valid}, {"refusal", valid}, {"end_turn", `{"dieu-1":{"codes":[]}}`}, {"end_turn", `{"dieu-1":{"codes":null},"dieu-2":{"codes":[]}}`}, {"end_turn", `{"dieu-1":{"codes":[]},"dieu-2":{"codes":[]},"extra":{"codes":[]}}`}, {"end_turn", `{"dieu-1":{"codes":[]},"dieu-1":{"codes":[]},"dieu-2":{"codes":[]}}`}, {"end_turn", `{"dieu-1":{"codes":[{"code":"1.1","confidence":null}]},"dieu-2":{"codes":[]}}`}, {"end_turn", valid + ` {}`}} {
		out := ValidateArticles(tc.stop, tc.text, req, articleNodes())
		for _, v := range out {
			if v.Outcome != "invalid_response" {
				t.Fatalf("accepted %s: %+v", tc.text, out)
			}
		}
	}
}

func TestValidateArticles_ConfidenceFailureIsPerArticle(t *testing.T) {
	req := ArticleRequest{Articles: articleInputs(2, ""), Nodes: articleNodes()[1:4]}
	for _, confidence := range []string{"-0.1", "1.1", "1e400"} {
		out := ValidateArticles("end_turn", `{"dieu-1":{"codes":[{"code":"1.1","confidence":`+confidence+`}]},"dieu-2":{"codes":[{"code":"1.1","confidence":1}]}}`, req, articleNodes())
		if out["dieu-1"].Outcome != "invalid_response" || out["dieu-2"].Outcome != "ok" {
			t.Fatalf("outcome %+v", out)
		}
	}
}
