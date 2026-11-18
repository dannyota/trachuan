package evidence

import (
	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/stage/build"
	"danny.vn/trachuan/apps/corpus/internal/stage/chunk"
	"danny.vn/trachuan/apps/corpus/internal/stage/relate"
	"danny.vn/trachuan/internal/retrieval"
)

func readClaims(documentKey string, rows []releaseProvision, incomplete bool, class string, cx claimContext) ([]claim, []string) {
	nodes := make([]chunk.Node, len(rows))
	byNode := make(map[string]releaseProvision, len(rows))
	nodeOf := make(map[int64]string, len(rows))
	children := make(map[int64][]int64, len(rows))
	for _, r := range rows {
		nodeOf[r.ID] = r.NodeID
		byNode[r.NodeID] = r
		if r.ParentID != nil {
			children[*r.ParentID] = append(children[*r.ParentID], r.ID)
		}
	}
	for i, r := range rows {
		nodes[i] = chunk.Node{NodeID: r.NodeID, Kind: r.Kind, Label: r.Label, Heading: r.Heading, Ordinal: r.Ordinal, Text: r.Text}
		if r.ParentID != nil {
			parent := nodeOf[*r.ParentID]
			nodes[i].ParentNodeID = &parent
		}
	}
	report := relate.ReadArticles(nodes, cx.issuers, incomplete)
	skipped := make([]string, len(report.Unread))
	for i, node := range report.Unread {
		skipped[i] = byNode[node].Anchor
	}
	var out []claim
	for _, reading := range report.Readings {
		article := byNode[reading.NodeID]
		for _, match := range reading.Matches {
			c := claim{document: documentKey, anchor: article.Anchor, subtree: subtree(article.ID, children), ordinal: match.Ordinal, class: class, severity: severityOf(class), kind: match.Kind, hash: sha(match.Sentence)}
			for _, target := range match.Targets {
				t := claimTarget{number: target.Number, anchor: target.Anchor}
				if ids := cx.numbers[retrieval.NormalizeNumber(target.Number)]; len(ids) == 1 {
					id := cx.identities[ids[0]]
					t.document = ids[0]
					t.ok = build.TextTargetIdentity(target.IssuerCode, target.StatedIssueDate, id.issuer, id.issueDate, cx.issuers)
				}
				c.targets = append(c.targets, t)
			}
			if len(c.targets) > 0 {
				out = append(out, c)
			}
		}
	}
	return out, skipped
}

func subtree(root int64, children map[int64][]int64) []int64 {
	out := []int64{root}
	for i := 0; i < len(out); i++ {
		out = append(out, children[out[i]]...)
	}
	return out
}

func severityOf(class string) detector.Severity {
	if class == classIncomplete {
		return detector.Info
	}
	return detector.Review
}
