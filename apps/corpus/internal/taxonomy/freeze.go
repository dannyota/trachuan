package taxonomy

import (
	"fmt"
	"sort"
)

func FreezePacket(packet Packet, labels []LabelArticle, labeler string) (FrozenLabels, error) {
	var out FrozenLabels
	if labeler != "gpt-6-astra" {
		return out, fmt.Errorf("labeler must be gpt-6-astra")
	}
	if packet.Country == "" || packet.Release == "" || packet.DrawRule == "" || (packet.Slice != "multi-family" && packet.Slice != "post-2023" && packet.Slice != "inheritance") {
		return out, fmt.Errorf("invalid packet identity")
	}
	nodes := map[string]bool{}
	levels := map[string]int{}
	for _, n := range packet.Tree {
		levels[n.Code] = n.Level
		if n.Code == "" || nodes[n.Code] {
			return out, fmt.Errorf("invalid packet tree")
		}
		if n.Level >= 2 && n.Level <= 4 {
			nodes[n.Code] = true
		}
	}
	byID := map[string]LabelArticle{}
	for _, a := range labels {
		id := a.Key + "\x00" + a.Anchor
		if a.Key == "" || a.Anchor == "" || len(a.Codes) == 0 {
			return out, fmt.Errorf("invalid article labels")
		}
		if _, ok := byID[id]; ok {
			return out, fmt.Errorf("duplicate article labels")
		}
		if a.NoNode && (len(a.Codes) != 1 || levels[a.Codes[0]] != 2) {
			return out, fmt.Errorf("no_node requires exactly one level-2 code")
		}
		seen := map[string]bool{}
		for _, c := range a.Codes {
			if !nodes[c] || seen[c] {
				return out, fmt.Errorf("unknown or duplicate code %q", c)
			}
			seen[c] = true
		}
		a.Codes = append([]string(nil), a.Codes...)
		sort.Strings(a.Codes)
		byID[id] = a
	}
	out = FrozenLabels{Country: packet.Country, Release: packet.Release, Seed: packet.Seed, DrawRule: packet.DrawRule, Labeler: labeler, Articles: make([]LabelArticle, 0, len(packet.Articles))}
	seen := map[string]bool{}
	for _, a := range packet.Articles {
		id := a.Key + "\x00" + a.Anchor
		if a.Key == "" || a.Anchor == "" || seen[id] || OwnTextSHA256(a.Text) != a.OwnTextSHA256 {
			return FrozenLabels{}, fmt.Errorf("invalid packet article")
		}
		seen[id] = true
		label, ok := byID[id]
		if !ok {
			return FrozenLabels{}, fmt.Errorf("missing labels for %s %s", a.Key, a.Anchor)
		}
		label.OwnTextSHA256 = a.OwnTextSHA256
		out.Articles = append(out.Articles, label)
		delete(byID, id)
	}
	if len(byID) > 0 || len(out.Articles) == 0 {
		return FrozenLabels{}, fmt.Errorf("extra or missing article labels")
	}
	sortArticles(out.Articles)
	return out, nil
}
