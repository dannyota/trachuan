package build

import "sort"

type partialActor struct {
	Key    string `json:"key"`
	Number string `json:"number"`
	Kind   string `json:"kind"`
}

type partialDocument struct {
	Key        string         `json:"key"`
	Number     string         `json:"number"`
	Class      string         `json:"class"`
	StatusCode string         `json:"status_code"`
	Acting     []partialActor `json:"acting"`
}

func partialDocuments(docs map[string]validityDocument, relations []validityRelation) []partialDocument {
	actors := make(map[string][]partialActor)
	for _, r := range relations {
		switch r.kind {
		case "repeals", "replaces", "suspends", "amends_supplements":
		default:
			continue
		}
		actor, ok := docs[r.source]
		if ok {
			actors[r.target] = append(actors[r.target], partialActor{Key: actor.key, Number: actor.number, Kind: r.kind})
		}
	}
	result := []partialDocument{}
	for _, d := range docs {
		if d.sourceClass != "partial" {
			continue
		}
		acting := actors[d.key]
		if acting == nil {
			acting = []partialActor{}
		}
		sort.Slice(acting, func(i, j int) bool {
			if acting[i].Key != acting[j].Key {
				return acting[i].Key < acting[j].Key
			}
			return acting[i].Kind < acting[j].Kind
		})
		result = append(result, partialDocument{Key: d.key, Number: d.number, Class: d.class, StatusCode: d.statusCode, Acting: acting})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key < result[j].Key })
	return result
}
