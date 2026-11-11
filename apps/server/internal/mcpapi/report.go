package mcpapi

import (
	"context"

	"danny.vn/trachuan/apps/server/internal/credential"
	"danny.vn/trachuan/apps/server/internal/reports"
	"danny.vn/trachuan/internal/retrieval"
	"github.com/google/jsonschema-go/jsonschema"
)

func (cfg Config) reportIssue(ctx context.Context, in reports.Input) (reports.Result, error) {
	principal, ok := credential.FromContext(ctx)
	if !ok || principal.AccountID <= 0 {
		return reports.Result{}, reports.ErrUnauthenticated
	}
	if cfg.Reports == nil {
		return reports.Result{}, reports.ErrUnavailable
	}
	return cfg.Reports.Submit(ctx, principal.AccountID, in)
}

func reportSchema() *jsonschema.Schema {
	s := &jsonschema.Schema{
		Type: "object", AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}},
		Required: []string{"country", "kind"},
		Properties: map[string]*jsonschema.Schema{
			"country":    {Type: "string", Enum: []any{"vn"}},
			"kind":       {Type: "string", Enum: []any{"wrong_text", "wrong_validity", "broken_citation", "missing_document"}},
			"handle":     {Type: "string", MinLength: ptrInt(1), MaxLength: ptrInt(retrieval.ReadHandleMax)},
			"release":    {Type: "string"},
			"suggestion": {Type: "object"},
		},
	}
	date := func() *jsonschema.Schema {
		return &jsonschema.Schema{Type: "string", Format: "date", MinLength: ptrInt(10), MaxLength: ptrInt(10)}
	}
	number := func() *jsonschema.Schema {
		return &jsonschema.Schema{Type: "string", MinLength: ptrInt(1), MaxLength: ptrInt(retrieval.ReadNumberMax)}
	}
	for _, kind := range []string{"wrong_text", "wrong_validity", "broken_citation", "missing_document"} {
		suggestion := &jsonschema.Schema{Type: "object", Properties: map[string]*jsonschema.Schema{}, AdditionalProperties: &jsonschema.Schema{Not: &jsonschema.Schema{}}}
		then := &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"suggestion": suggestion}}
		if kind != "missing_document" {
			then.Required = []string{"handle"}
		}
		switch kind {
		case "wrong_validity":
			suggestion.Properties = map[string]*jsonschema.Schema{
				"status_class": {Type: "string", Enum: []any{"in_force", "partial", "expired", "not_yet", "suspended", "unknown"}},
				"eff_from":     date(), "eff_to": date(),
			}
		case "broken_citation":
			suggestion.Properties["document_number"] = number()
		case "missing_document":
			then.Required = []string{"suggestion"}
			suggestion.Required = []string{"document_number"}
			suggestion.Properties["document_number"] = number()
			suggestion.Properties["issue_date"] = date()
		}
		s.AllOf = append(s.AllOf, &jsonschema.Schema{
			If: &jsonschema.Schema{Properties: map[string]*jsonschema.Schema{"kind": {Enum: []any{kind}}}}, Then: then,
		})
	}
	return s
}
