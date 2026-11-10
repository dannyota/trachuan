package httpapi

import (
	"encoding/json"
	"strings"
)

func failure(status int, code string, count *int) response {
	type detail struct {
		Code       string `json:"code"`
		Message    string `json:"message"`
		Index      *int   `json:"index"`
		TokenCount *int   `json:"token_count"`
	}
	var index *int
	if count != nil {
		zero := 0
		index = &zero
	}
	b, _ := json.Marshal(struct {
		Error detail `json:"error"`
	}{detail{code, strings.ReplaceAll(code, "_", " "), index, count}})
	return response{status: status, code: code, payload: b}
}
