package taxonomy

import (
	"fmt"
	"math/rand"
	"sort"
)

const AdjudicatedDrawRule = "model-adjudicated/1"

type PacketArticle struct {
	Key           string `json:"document_key"`
	Anchor        string `json:"anchor"`
	OwnTextSHA256 string `json:"own_text_sha256"`
	Text          string `json:"text"`
	Number        string `json:"number"`
	Title         string `json:"title"`
}
type PacketNode struct {
	Code       string `json:"code"`
	ParentCode string `json:"parent_code"`
	Level      int    `json:"level"`
	Name       string `json:"name"`
}
type Packet struct {
	Country  string          `json:"country"`
	Release  string          `json:"release"`
	Slice    string          `json:"slice"`
	Seed     int64           `json:"seed"`
	DrawRule string          `json:"draw_rule"`
	Articles []PacketArticle `json:"articles"`
	Tree     []PacketNode    `json:"tree"`
}
type DrawArticle struct {
	PacketArticle
	Primary, Seeded, MultiFamily, Inherited bool
	IssueDate                               string
}
type FrozenLabels struct {
	Country  string         `json:"country"`
	Release  string         `json:"release"`
	Seed     int64          `json:"seed"`
	DrawRule string         `json:"draw_rule"`
	Labeler  string         `json:"labeler"`
	Articles []LabelArticle `json:"articles"`
}

func DrawAdjudicated(rows []DrawArticle, slice string, seed int64) ([]PacketArticle, error) {
	if slice != "multi-family" && slice != "post-2023" && slice != "inheritance" {
		return nil, fmt.Errorf("unknown slice %q", slice)
	}
	pool := make([]DrawArticle, 0, len(rows))
	for _, a := range rows {
		if a.Seeded {
			continue
		}
		switch slice {
		case "multi-family":
			if !a.Primary {
				continue
			}
		case "post-2023":
			if !a.Primary || a.IssueDate < "2024-01-01" {
				continue
			}
		case "inheritance":
			if !a.Inherited {
				continue
			}
		}
		pool = append(pool, a)
	}
	sort.Slice(pool, func(i, j int) bool {
		if pool[i].Key == pool[j].Key {
			return pool[i].Anchor < pool[j].Anchor
		}
		return pool[i].Key < pool[j].Key
	})
	r := rand.New(rand.NewSource(seed))
	r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	counts := map[string]int{}
	used := map[string]bool{}
	out := make([]PacketArticle, 0, 100)
	take := func(a DrawArticle) {
		counts[a.Key]++
		used[a.Key+"\x00"+a.Anchor] = true
		a.OwnTextSHA256 = OwnTextSHA256(a.Text)
		out = append(out, a.PacketArticle)
	}
	if slice == "multi-family" {
		for _, a := range pool {
			if a.MultiFamily && counts[a.Key] < 2 {
				take(a)
				if len(out) == 50 {
					break
				}
			}
		}
		if len(out) < 50 {
			return nil, fmt.Errorf("not enough multi-family eligible articles: got %d, need 50", len(out))
		}
	}
	if slice == "multi-family" {
		r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	}
	for _, a := range pool {
		if counts[a.Key] >= 2 || used[a.Key+"\x00"+a.Anchor] {
			continue
		}
		take(a)
		if len(out) == 100 {
			break
		}
	}
	if len(out) < 100 {
		return nil, fmt.Errorf("not enough eligible articles: got %d, need 100", len(out))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].Anchor < out[j].Anchor
		}
		return out[i].Key < out[j].Key
	})
	return out, nil
}
