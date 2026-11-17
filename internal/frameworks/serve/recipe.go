package serve

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"

	"danny.vn/trachuan/internal/retrieval"
)

type recipe struct {
	Version      int                 `json:"version"`
	Status       string              `json:"status"`
	DenseDepth   int                 `json:"dense_depth"`
	LexicalDepth int                 `json:"lexical_depth"`
	RRF          retrieval.RRFRecipe `json:"rrf"`
	Abstention   struct {
		Statistic         string   `json:"statistic"`
		Threshold         *float64 `json:"threshold"`
		Status            string   `json:"status"`
		CalibrationSHA256 string   `json:"calibration_sha256,omitempty"`
	} `json:"abstention"`
	Source struct {
		Release           string `json:"release"`
		RecipeVersion     int    `json:"recipe_version"`
		CalibrationSHA256 string `json:"calibration_sha256"`
	} `json:"source"`
}

func decodeRecipe(raw []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if dec.Decode(out) != nil || dec.Decode(new(any)) != io.EOF {
		return ErrUnavailable
	}
	return nil
}

func (r recipe) validate() error {
	if r.Version != 2 || r.DenseDepth < 1 || r.DenseDepth > 200 || r.LexicalDepth < 1 || r.LexicalDepth > 100 || r.RRF.K < 1 || r.Abstention.Statistic != "first-hit-best-permitted-cosine" || r.Abstention.Threshold == nil || r.Source.Release == "" || r.Source.RecipeVersion != 3 {
		return ErrUnavailable
	}
	switch r.Status {
	case "placeholder":
		if r.Abstention.Status != "bootstrap-from-law" || r.Abstention.CalibrationSHA256 != "" {
			return ErrUnavailable
		}
	case "measured":
		hash, err := hex.DecodeString(r.Abstention.CalibrationSHA256)
		if r.Abstention.Status != "measured" || err != nil || len(hash) != 32 {
			return ErrUnavailable
		}
	default:
		return ErrUnavailable
	}
	hash, err := hex.DecodeString(r.Source.CalibrationSHA256)
	if err != nil || len(hash) != 32 {
		return ErrUnavailable
	}
	for _, value := range []float64{r.RRF.DenseWeight, r.RRF.LexicalWeight, r.RRF.RoutedLexicalWeight} {
		if !finite(value) || value <= 0 {
			return ErrUnavailable
		}
	}
	if threshold := *r.Abstention.Threshold; !finite(threshold) || threshold < -1 || threshold > 2 {
		return ErrUnavailable
	}
	return nil
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
