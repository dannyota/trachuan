package evaluate

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"danny.vn/trachuan/internal/frameworks/serve"
)

type invariantObservation struct {
	Operation, CaseID, Mode string
	Page, Repeat            int
	Value                   []byte
	Scores                  []byte
	Evidence                []byte
	Text                    []byte
}

func observe(ctx context.Context, e EvaluationEngine, req EvaluationRequest, caller serve.Caller) ([]invariantObservation, error) {
	var all []invariantObservation
	add := func(op, id, mode string, page int, value, scores, evidence, text any, again func() (any, error)) error {
		raw, err := json.Marshal(value)
		if err != nil {
			return hide("encode invariant response", err)
		}
		s, _ := json.Marshal(scores)
		v, _ := json.Marshal(evidence)
		t, _ := json.Marshal(text)
		replayed, replayErr := again()
		if replayErr != nil {
			return &caseFailure{id, "invariant " + op + " replay failed", replayErr}
		}
		replayRaw, marshalErr := json.Marshal(replayed)
		if marshalErr != nil {
			return hide("encode invariant replay", marshalErr)
		}
		if !bytes.Equal(raw, replayRaw) {
			return &caseFailure{id, "invariant " + op + " cache response changed", nil}
		}
		all = append(all, invariantObservation{op, id, mode, page, 0, raw, s, v, t}, invariantObservation{op, id, mode, page, 1, replayRaw, s, v, t})
		return nil
	}

	for _, c := range req.Cases {
		for _, mode := range []string{"open", "filtered"} {
			if c.Negative && mode == "filtered" {
				continue
			}
			r := caseRequest(c, mode)
			seen := map[string]bool{}
			for page := 0; ; page++ {
				response, err := e.Search(ctx, r, caller)
				if err != nil {
					if addErr := add("search", c.ID, mode, page, struct {
						Response serve.Response
						Error    string
					}{response, err.Error()}, nil, nil, nil, func() (any, error) {
						repeatedResponse, repeatedErr := e.Search(ctx, r, caller)
						message := ""
						if repeatedErr != nil {
							message = repeatedErr.Error()
						}
						return struct {
							Response serve.Response
							Error    string
						}{repeatedResponse, message}, nil
					}); addErr != nil {
						return nil, addErr
					}
					break
				}
				scores := make([]any, 0, len(response.Hits))
				for _, hit := range response.Hits {
					scores = append(scores, struct {
						Handle string
						Score  *float64
					}{hit.Handle, hit.Score})
				}
				if err = add("search", c.ID, mode, page, response, scores, nil, nil, func() (any, error) { return e.Search(ctx, r, caller) }); err != nil {
					return nil, err
				}
				if response.NextCursor == "" {
					break
				}
				if seen[response.NextCursor] {
					return nil, &caseFailure{c.ID, "invariant search continuation repeated", nil}
				}
				seen[response.NextCursor] = true
				r.Cursor = response.NextCursor
			}
		}
	}
	targets := map[string]string{}
	for _, c := range req.Cases {
		if c.Target != "" {
			targets[c.Target] = c.ID
		}
	}
	for _, c := range req.Controls {
		if _, exists := targets[controlHandle(c)]; !exists {
			targets[controlHandle(c)] = ""
		}
	}
	handles := make([]string, 0, len(targets))
	for h := range targets {
		handles = append(handles, h)
	}
	slices.Sort(handles)
	for _, h := range handles {
		id := targets[h]
		r := serve.ReadRequest{Items: []serve.ReadItem{{Handle: h}}}
		seen := map[string]bool{}
		for page := 0; ; page++ {
			response, err := e.Read(ctx, r, caller)
			if err != nil {
				return nil, &caseFailure{id, "invariant read failed", err}
			}
			if len(response.Items) != 1 || response.Items[0].Error != "" || response.Items[0].NotRead != "" {
				return nil, &caseFailure{id, "invariant read item failed", nil}
			}
			texts := []string{}
			evidence := []string{}
			for _, unit := range response.Items[0].Units {
				if unit.Text != "" {
					texts = append(texts, unit.Text)
				}
				if unit.Edge != nil {
					evidence = append(evidence, unit.Edge.Evidence)
				}
			}
			if err = add("read", id, h, page, response, nil, evidence, texts, func() (any, error) { return e.Read(ctx, r, caller) }); err != nil {
				return nil, err
			}
			next := response.Items[0].Next
			if next == "" {
				if !response.Items[0].Complete {
					return nil, &caseFailure{id, "invariant incomplete read", nil}
				}
				break
			}
			if seen[next] {
				return nil, &caseFailure{id, "invariant read continuation repeated", nil}
			}
			seen[next] = true
			r.Items = []serve.ReadItem{{Next: next}}
		}
		m := serve.MappingsRequest{Handle: h, Limit: 8}
		seen = map[string]bool{}
		for page := 0; ; page++ {
			response, err := e.Mappings(ctx, m, caller)
			if err != nil {
				return nil, &caseFailure{id, "invariant mappings failed", err}
			}
			values := []string{}
			for _, edge := range response.Edges {
				values = append(values, edge.Evidence)
			}
			if err = add("mappings", id, h, page, response, nil, values, nil, func() (any, error) { return e.Mappings(ctx, m, caller) }); err != nil {
				return nil, err
			}
			if response.NextCursor == "" {
				break
			}
			if seen[response.NextCursor] {
				return nil, &caseFailure{id, "invariant mappings continuation repeated", nil}
			}
			seen[response.NextCursor] = true
			m.Cursor = response.NextCursor
		}
	}
	status, err := e.CorpusStatus(ctx, serve.StatusRequest{}, caller)
	if err != nil {
		return nil, hide("invariant status failed", err)
	}
	if err = add("corpus_status", "", "", 0, status, nil, nil, nil, func() (any, error) { return e.CorpusStatus(ctx, serve.StatusRequest{}, caller) }); err != nil {
		return nil, err
	}
	r := serve.GapsRequest{}
	seen := map[string]bool{}
	for page := 0; ; page++ {
		response, err := e.QualityGaps(ctx, r, caller)
		if err != nil {
			return nil, hide("invariant gaps failed", err)
		}
		if err = add("quality_gaps", "", "", page, response, nil, nil, nil, func() (any, error) { return e.QualityGaps(ctx, r, caller) }); err != nil {
			return nil, err
		}
		if response.NextCursor == "" {
			break
		}
		if seen[response.NextCursor] {
			return nil, fmt.Errorf("invariant gaps continuation repeated")
		}
		seen[response.NextCursor] = true
		r.Cursor = response.NextCursor
	}
	return all, nil
}
