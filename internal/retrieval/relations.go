package retrieval

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

func shapeRelations(source []SourceRelation, recipe Recipe, id Identity) ([]Relation, []Gap) {
	edges := append([]SourceRelation(nil), source...)
	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.Value.Kind != b.Value.Kind {
			return a.Value.Kind < b.Value.Kind
		}
		if a.Value.SourceKey != b.Value.SourceKey {
			return a.Value.SourceKey < b.Value.SourceKey
		}
		ak, bk := "", ""
		if a.Value.TargetKey != nil {
			ak = *a.Value.TargetKey
		}
		if b.Value.TargetKey != nil {
			bk = *b.Value.TargetKey
		}
		if ak != bk {
			return ak < bk
		}
		if a.ArtifactSHA256 != b.ArtifactSHA256 {
			return a.ArtifactSHA256 < b.ArtifactSHA256
		}
		return a.Ordinal < b.Ordinal
	})
	result := []Relation{}
	// An edge is identified by its source key, artifact hash and ordinal:
	// two documents can have byte-identical relation artifacts.
	type edgeID struct {
		source, artifact string
		ordinal          int
	}
	seen := map[edgeID]bool{}
	omitted := false
	for _, e := range edges {
		id := edgeID{e.Value.SourceKey, e.ArtifactSHA256, e.Ordinal}
		if seen[id] {
			continue
		}
		seen[id] = true
		if len(result) >= recipe.RelationCap {
			omitted = true
			continue
		}
		r := e.Value
		r.SourceInHit, r.TargetInHit = e.SourceInHit, e.TargetInHit
		sum := sha256.Sum256([]byte(r.Evidence))
		r.EvidenceSHA256 = hex.EncodeToString(sum[:])
		chars := []rune(r.Evidence)
		r.Complete = len(chars) <= recipe.RelationEvidenceRuneCap
		if !r.Complete {
			r.Evidence = string(chars[:recipe.RelationEvidenceRuneCap])
		}
		result = append(result, r)
	}
	var gaps []Gap
	if omitted {
		gaps = append(gaps, Gap{Kind: "relations_omitted", DocumentKey: id.DocumentKey, Anchor: id.Anchor, Message: "Read the document for omitted relations."})
	}
	return result, gaps
}
