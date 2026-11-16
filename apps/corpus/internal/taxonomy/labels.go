package taxonomy

import (
	"crypto/sha256"
	"fmt"
	"math/rand"
	"sort"
)

const DrawRuleVersion = "seeded-articles/2"

type LabelArticle struct {
	Key           string   `json:"document_key"`
	Anchor        string   `json:"anchor"`
	OwnTextSHA256 string   `json:"own_text_sha256"`
	Codes         []string `json:"codes"`
}

type Labels struct {
	Country  string         `json:"country"`
	Release  string         `json:"release"`
	Seed     int64          `json:"seed"`
	DrawRule string         `json:"draw_rule"`
	Articles []LabelArticle `json:"articles"`
}

func OwnTextSHA256(text string) string {
	sum := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%x", sum)
}

func DrawLabels(input []LabelArticle, seed int64) ([]LabelArticle, error) {
	pool := append([]LabelArticle(nil), input...)
	sortArticles(pool)
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	counts := map[string]int{}
	out := make([]LabelArticle, 0, 300)
	for _, article := range pool {
		if counts[article.Key] >= 2 {
			continue
		}
		counts[article.Key]++
		out = append(out, article)
		if len(out) == 300 {
			break
		}
	}
	if len(out) < 300 {
		return nil, fmt.Errorf("not enough eligible articles: got %d, need 300", len(out))
	}
	sortArticles(out)
	return out, nil
}

func sortArticles(articles []LabelArticle) {
	sort.Slice(articles, func(i, j int) bool {
		if articles[i].Key == articles[j].Key {
			return articles[i].Anchor < articles[j].Anchor
		}
		return articles[i].Key < articles[j].Key
	})
}
