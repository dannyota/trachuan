package classify

import (
	"encoding/json"
	"math"
)

func validateMenuArticles(stop, text string, req ArticleRequest) (map[string]ArticleOutcome, int) {
	invalid := map[string]ArticleOutcome{}
	for _, a := range req.Articles {
		invalid[a.Anchor] = emptyArticle("invalid_response")
	}
	if stop != "end_turn" {
		return invalid, 0
	}
	raw, err := readObject([]byte(text))
	if err != nil {
		return invalid, 0
	}
	dropped := 0
	for anchor := range raw {
		if _, ok := invalid[anchor]; !ok {
			delete(raw, anchor)
			dropped++
		}
	}
	if len(raw) != len(invalid) {
		return invalid, dropped
	}
	allowed := map[string]bool{}
	for _, n := range req.Nodes {
		allowed[n.Code] = true
	}
	result := map[string]ArticleOutcome{}
	for anchor, value := range raw {
		obj, err := closedObject(value, []string{"primary", "additional"})
		if err != nil {
			return invalid, dropped
		}
		primary, ok := menuConfidence(obj["primary"])
		if !ok {
			return invalid, dropped
		}
		var additional CodeConfidence
		hasAdditional := string(obj["additional"]) != "null"
		if hasAdditional {
			additional, ok = menuConfidence(obj["additional"])
			if !ok {
				return invalid, dropped
			}
		}
		out := emptyArticle("ok")
		if !allowed[primary.Code] {
			out.Outcome = "invalid_response"
			out.Dropped = append(out.Dropped, primary.Code)
		} else {
			out.Codes = append(out.Codes, primary)
		}
		if hasAdditional {
			if !allowed[additional.Code] || additional.Code == primary.Code {
				out.Dropped = append(out.Dropped, additional.Code)
			} else if out.Outcome == "ok" {
				out.Codes = append(out.Codes, additional)
			}
		}
		result[anchor] = out
	}
	return result, dropped
}

func menuConfidence(raw json.RawMessage) (CodeConfidence, bool) {
	var result CodeConfidence
	obj, err := closedObject(raw, []string{"code", "confidence"})
	if err != nil || len(obj["code"]) == 0 || obj["code"][0] != '"' || json.Unmarshal(obj["code"], &result.Code) != nil || string(obj["confidence"]) == "null" || json.Unmarshal(obj["confidence"], &result.Confidence) != nil || math.IsNaN(result.Confidence) || math.IsInf(result.Confidence, 0) || result.Confidence < 0 || result.Confidence > 1 {
		return result, false
	}
	return result, true
}
