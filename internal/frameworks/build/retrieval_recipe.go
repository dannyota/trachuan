package build

import "danny.vn/trachuan/internal/retrieval"

func bootstrapRetrievalRecipe() map[string]any {
	law := retrieval.Recipe3()
	return map[string]any{
		"version": 2, "status": "placeholder", "dense_depth": law.DenseDepth, "lexical_depth": law.LexicalDepth,
		"rrf": law.RRF,
		"abstention": map[string]any{
			"statistic": "first-hit-best-permitted-cosine", "threshold": 0.5297041131124263,
			"status": "bootstrap-from-law",
		},
		"source": map[string]any{
			"release": "vn/20261115_3", "recipe_version": 3,
			"calibration_sha256": "9264612fa2d933e3f0b1ee451e7707598192f9c66ed52224d48a9a327ca65113",
		},
	}
}

func measuredRetrievalRecipe(threshold float64, calibrationHash string) map[string]any {
	r := bootstrapRetrievalRecipe()
	r["status"] = "measured"
	a := r["abstention"].(map[string]any)
	a["threshold"], a["calibration_sha256"], a["status"] = threshold, calibrationHash, "measured"
	return r
}
