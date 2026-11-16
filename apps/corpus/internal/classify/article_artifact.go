package classify

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
)

type FamilyArtifact struct {
	Version      int                `json:"version"`
	RecipeHash   string             `json:"recipe_hash"`
	TaxonomyHash string             `json:"taxonomy_hash"`
	Recipe       Recipe             `json:"recipe"`
	Documents    map[string]Outcome `json:"documents"`
}

type ArticleMetadata struct {
	LabelsHash string          `json:"labels_sha256"`
	Skipped    []ArticleRecord `json:"skipped"`
}

type ArticleArtifact struct {
	Version        int             `json:"version"`
	Country        string          `json:"country"`
	Release        string          `json:"release"`
	Recipe         Recipe          `json:"recipe"`
	RecipeHash     string          `json:"recipe_hash"`
	TaxonomyHash   string          `json:"taxonomy_hash"`
	FamiliesHash   string          `json:"families_sha256"`
	LabelsHash     string          `json:"labels_sha256"`
	Articles       []ArticleRecord `json:"articles"`
	DroppedAnchors int             `json:"dropped_anchors"`
}

func readArtifact(path string, dst any) (string, error) {
	dir, name := filepath.Dir(path), filepath.Base(path)
	if _, err := artifact.VerifyFiles(dir, name); err != nil {
		return "", fmt.Errorf("verify classification artifact: %w", err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read classification artifact: %w", err)
	}
	if err = json.Unmarshal(b, dst); err != nil {
		return "", fmt.Errorf("decode classification artifact: %w", err)
	}
	return digest(b), nil
}

func ReadFamilyArtifact(path, taxonomyHash string, nodes []Node) (FamilyArtifact, string, error) {
	var f FamilyArtifact
	hash, err := readArtifact(path, &f)
	if err != nil {
		return f, "", err
	}
	if f.Version != 1 || f.Recipe.Validate() != nil || f.Recipe.Article != nil || f.Recipe.Hash() != f.RecipeHash || f.TaxonomyHash != taxonomyHash || f.Recipe.TaxonomyHash != taxonomyHash || len(f.Documents) == 0 {
		return f, "", fmt.Errorf("families artifact recipe or taxonomy mismatch")
	}
	for key, out := range f.Documents {
		if key == "" || len(out.Codes) > f.Recipe.F || (out.Outcome != "ok" && out.Outcome != "no_family" && out.Outcome != "invalid_response") || (out.Outcome == "ok") != (len(out.Codes) > 0) {
			return f, "", fmt.Errorf("invalid family artifact outcome")
		}
		seen := map[string]bool{}
		for _, code := range out.Codes {
			if seen[code] {
				return f, "", fmt.Errorf("duplicate family artifact code")
			}
			seen[code] = true
		}
		if _, err = allowedNodes(out.Codes, nodes); err != nil {
			return f, "", err
		}
	}
	return f, hash, nil
}

func ReadArticleArtifact(path, taxonomyHash, familiesHash string) (ArticleArtifact, error) {
	var a ArticleArtifact
	if _, err := readArtifact(path, &a); err != nil {
		return a, err
	}
	if a.Version != 1 || a.Recipe.Validate() != nil || a.Recipe.Article == nil || a.RecipeHash != a.Recipe.Hash() || a.TaxonomyHash != taxonomyHash || a.Recipe.TaxonomyHash != taxonomyHash || a.FamiliesHash != familiesHash || a.Recipe.Article.FamiliesHash != familiesHash {
		return a, fmt.Errorf("articles artifact recipe, taxonomy or families mismatch")
	}
	seen := map[string]bool{}
	for _, record := range a.Articles {
		key := record.DocumentKey + "/" + record.Anchor
		if record.DocumentKey == "" || record.Anchor == "" || seen[key] {
			return a, fmt.Errorf("invalid or duplicate article artifact identity")
		}
		seen[key] = true
		if err := validateArticleOutcome(record.Outcome); err != nil {
			return a, err
		}
	}
	return a, nil
}

func validateArticleOutcome(out ArticleOutcome) error {
	switch out.Outcome {
	case "ok":
	case "no_family", "too_long", "invalid_response":
		if len(out.Codes) > 0 {
			return fmt.Errorf("failed article artifact outcome carries codes")
		}
	default:
		return fmt.Errorf("unknown article artifact outcome")
	}
	seen := map[string]bool{}
	for _, code := range out.Codes {
		if code.Code == "" || seen[code.Code] || math.IsNaN(code.Confidence) || math.IsInf(code.Confidence, 0) || code.Confidence < 0 || code.Confidence > 1 {
			return fmt.Errorf("invalid article artifact code or confidence")
		}
		seen[code.Code] = true
	}
	return nil
}

func writeArticles(dir string, s Submission) error {
	if s.Recipe.Article == nil {
		return fmt.Errorf("article recipe is required")
	}
	a := ArticleArtifact{Version: 1, Country: s.Country, Release: s.Release, Recipe: s.Recipe, RecipeHash: s.Recipe.Hash(), TaxonomyHash: s.Recipe.TaxonomyHash, FamiliesHash: s.Recipe.Article.FamiliesHash, Articles: []ArticleRecord{}}
	for _, item := range s.Items {
		a.DroppedAnchors += s.Results[item.Key].DroppedAnchors
		if item.ArticleMeta != nil {
			a.LabelsHash = item.ArticleMeta.LabelsHash
			a.Articles = append(a.Articles, item.ArticleMeta.Skipped...)
		}
		if item.Article == nil {
			continue
		}
		for _, article := range item.Article.Articles {
			out, ok := s.Results[item.Key].Articles[article.Anchor]
			if !ok {
				out = emptyArticle("invalid_response")
			}
			a.Articles = append(a.Articles, ArticleRecord{item.Article.DocumentKey, article.Anchor, out})
		}
	}
	sort.Slice(a.Articles, func(i, j int) bool {
		if a.Articles[i].DocumentKey != a.Articles[j].DocumentKey {
			return a.Articles[i].DocumentKey < a.Articles[j].DocumentKey
		}
		return a.Articles[i].Anchor < a.Articles[j].Anchor
	})
	b, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("encode articles artifact: %w", err)
	}
	b = append(b, '\n')
	if err = writeOnce(filepath.Join(dir, "articles.json"), b); err != nil {
		return err
	}
	return writeOnce(filepath.Join(dir, "MANIFEST.sha256"), []byte(digest(b)+"  articles.json\n"))
}
