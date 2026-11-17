package serve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

type calibrationObservation struct {
	cosine *float64
	lookup bool
}

func (e *Engine) CalibrationSearch(ctx context.Context, query string) (*float64, bool, error) {
	observation := &calibrationObservation{}
	_, err := e.search(ctx, Request{Query: query, Limit: 8}, Caller{Readable: func(_ string, access string) bool { return access == "free" }}, observation)
	if err != nil {
		return nil, false, err
	}
	return observation.cosine, observation.lookup, nil
}

func (e *Engine) CalibrationRecipeHash() string {
	r := e.recipe
	r.Status, r.Abstention.Status = "", ""
	r.Abstention.Threshold, r.Abstention.CalibrationSHA256 = nil, ""
	encoded, _ := json.Marshal(struct {
		Retrieval recipe `json:"retrieval"`
		Dense     any    `json:"dense"`
		Lexical   any    `json:"lexical"`
	}{r, e.dense, e.lexical})
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}
