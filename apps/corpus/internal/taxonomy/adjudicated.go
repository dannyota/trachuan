package taxonomy

import (
	"fmt"
	"math/rand"
	"sort"
)

const AdjudicatedDrawRule = "model-adjudicated/1"

//nolint:misspell // The labeling rule must match the design verbatim.
const LabelingRule = "Assign a node only when the article's own operative text regulates that subject. A mention, a cross-reference, the agency responsible, or a document type named in passing (a contract, a report, a copy, a licence) is not enough. One family is the default; add another family only when a substantive part of the article is about it."

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
type ArticleID struct {
	Key    string `json:"document_key"`
	Anchor string `json:"anchor"`
}

type Packet struct {
	LabelerFragments int             `json:"labeler_fragments"`
	LabelingRule     string          `json:"labeling_rule"`
	FragmentsSkipped int             `json:"fragments_skipped"`
	Country          string          `json:"country"`
	Release          string          `json:"release"`
	Slice            string          `json:"slice"`
	Seed             int64           `json:"seed"`
	DrawRule         string          `json:"draw_rule"`
	Articles         []PacketArticle `json:"articles"`
	Tree             []PacketNode    `json:"tree"`
}
type DrawArticle struct {
	PacketArticle
	Primary, Seeded, MultiFamily, Inherited bool
	IssueDate                               string
}
type FrozenLabels struct {
	LabelerFragments int            `json:"labeler_fragments"`
	Country          string         `json:"country"`
	Release          string         `json:"release"`
	Seed             int64          `json:"seed"`
	DrawRule         string         `json:"draw_rule"`
	Labeler          string         `json:"labeler"`
	Articles         []LabelArticle `json:"articles"`
}

func DrawAdjudicated(rows []DrawArticle, slice string, seed int64) ([]PacketArticle, error) {
	out, _, err := DrawAdjudicatedWithSkipped(rows, slice, seed)
	return out, err
}

func DrawAdjudicatedWithSkipped(rows []DrawArticle, slice string, seed int64) ([]PacketArticle, int, error) {
	out, skipped, _, err := DrawAdjudicatedExcluding(rows, slice, seed, nil)
	return out, skipped, err
}

func DrawAdjudicatedExcluding(rows []DrawArticle, slice string, seed int64, exclusions []ArticleID) ([]PacketArticle, int, int, error) {
	if slice != "multi-family" && slice != "post-2023" && slice != "inheritance" {
		return nil, 0, 0, fmt.Errorf("unknown slice %q", slice)
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
	familyPool := append([]DrawArticle(nil), pool...)
	familyNext, randomNext := 0, 0
	counts := map[string]int{}
	used := map[string]bool{}
	out := make([]PacketArticle, 0, 100)
	skipped := map[string]bool{}
	fragment := func(a DrawArticle) bool {
		if !IsFragment(a.Text) {
			return false
		}
		skipped[a.Key+"\x00"+a.Anchor] = true
		return true
	}
	take := func(a DrawArticle) {
		counts[a.Key]++
		used[a.Key+"\x00"+a.Anchor] = true
		a.OwnTextSHA256 = OwnTextSHA256(a.Text)
		out = append(out, a.PacketArticle)
	}
	if slice == "multi-family" {
		for i, a := range pool {
			familyNext = i + 1
			if a.MultiFamily && counts[a.Key] < 2 && !fragment(a) {
				take(a)
				if len(out) == 50 {
					break
				}
			}
		}
		if len(out) < 50 {
			return nil, 0, 0, fmt.Errorf("not enough multi-family eligible articles: got %d, need 50", len(out))
		}
	}
	if slice == "multi-family" {
		r.Shuffle(len(pool), func(i, j int) { pool[i], pool[j] = pool[j], pool[i] })
	}
	for i, a := range pool {
		randomNext = i + 1
		if counts[a.Key] >= 2 || used[a.Key+"\x00"+a.Anchor] || fragment(a) {
			continue
		}
		take(a)
		if len(out) == 100 {
			break
		}
	}
	if len(out) < 100 {
		return nil, 0, 0, fmt.Errorf("not enough eligible articles: got %d, need 100", len(out))
	}
	excluded := map[string]bool{}
	for _, a := range exclusions {
		id := a.Key + "\x00" + a.Anchor
		if a.Key == "" || a.Anchor == "" || excluded[id] {
			return nil, 0, 0, fmt.Errorf("invalid or duplicate excluded article")
		}
		excluded[id] = true
	}
	removed := map[string]bool{}
	familyIDs := map[string]bool{}
	if slice == "multi-family" {
		for _, a := range familyPool[:familyNext] {
			id := a.Key + "\x00" + a.Anchor
			if a.MultiFamily && used[id] {
				familyIDs[id] = true
			}
		}
	}
	familyMissing := 0
	kept := out[:0]
	for _, a := range out {
		id := a.Key + "\x00" + a.Anchor
		if excluded[id] {
			removed[id] = true
			counts[a.Key]--
			delete(used, id)
			if familyIDs[id] {
				familyMissing++
			}
			continue
		}
		kept = append(kept, a)
	}
	out = kept
	fill := func(candidates []DrawArticle, family bool) {
		for _, a := range candidates {
			if (family && familyMissing == 0) || (!family && len(out) == 100) {
				break
			}
			id := a.Key + "\x00" + a.Anchor
			if (family && !a.MultiFamily) || counts[a.Key] >= 2 || used[id] {
				continue
			}
			if excluded[id] {
				removed[id] = true
				continue
			}
			if fragment(a) {
				continue
			}
			take(a)
			if family {
				familyMissing--
			}
		}
	}
	if familyMissing > 0 {
		fill(familyPool[familyNext:], true)
	}
	if familyMissing > 0 {
		return nil, 0, 0, fmt.Errorf("not enough multi-family replacements")
	}
	if len(out) < 100 {
		fill(pool[randomNext:], false)
	}
	if len(out) < 100 {
		return nil, 0, 0, fmt.Errorf("not enough replacement articles: got %d, need 100", len(out))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Key == out[j].Key {
			return out[i].Anchor < out[j].Anchor
		}
		return out[i].Key < out[j].Key
	})
	return out, len(skipped), len(removed), nil
}
