package evaluate

import (
	"bytes"
	"context"
	"encoding/json"

	"danny.vn/trachuan/internal/frameworks/serve"
)

func observeSearch(ctx context.Context, e EvaluationEngine, request serve.Request, caller serve.Caller, c GoldenCase, mode string, page int) ([]invariantObservation, serve.Response, error) {
	response, firstErr := e.Search(ctx, request, caller)
	replay, replayErr := e.Search(ctx, request, caller)
	diagnostic := c.Adjudication != "accepted"
	observations := []invariantObservation{
		{Operation: "search", CaseID: c.ID, Mode: mode, Page: page, Repeat: 0, Diagnostic: diagnostic},
		{Operation: "search", CaseID: c.ID, Mode: mode, Page: page, Repeat: 1, Diagnostic: diagnostic},
	}
	if firstErr != nil || replayErr != nil {
		if !diagnostic {
			if firstErr == nil {
				firstErr = replayErr
			}
			return nil, response, &caseFailure{c.ID, "invariant search failed", firstErr}
		}
		observations[0].Error, observations[1].Error = true, true
		return observations, response, nil
	}
	for i, value := range []serve.Response{response, replay} {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, response, hide("encode invariant search", err)
		}
		scores := make([]any, 0, len(value.Hits))
		for _, hit := range value.Hits {
			scores = append(scores, struct {
				Handle string
				Score  *float64
			}{hit.Handle, hit.Score})
		}
		encoded, err := json.Marshal(scores)
		if err != nil {
			return nil, response, hide("encode invariant scores", err)
		}
		observations[i].Value, observations[i].Scores = raw, encoded
	}
	if !bytes.Equal(observations[0].Value, observations[1].Value) {
		return nil, response, &caseFailure{c.ID, "invariant search cache response changed", nil}
	}
	return observations, response, nil
}
