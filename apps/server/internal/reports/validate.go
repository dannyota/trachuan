package reports

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"danny.vn/trachuan/internal/retrieval"
)

// Validate resolves and canonicalizes every report field against the corpus.
// The caller supplies the release selected by Resolver.
func Validate(ctx context.Context, corpus Corpus, in Input, now time.Time) (Report, error) {
	if err := validateFields(in); err != nil {
		return Report{}, err
	}
	if corpus == nil {
		return Report{}, ErrUnavailable
	}
	report := Report{GroupKey: GroupKey{Kind: in.Kind}, Release: in.Release}
	if in.Handle != "" {
		key, anchor, _ := retrieval.ParseHandle(in.Handle)
		doc, exists, err := corpus.Document(ctx, key)
		if err != nil {
			return Report{}, corpusFailure(err)
		}
		if !exists {
			return Report{}, ErrInvalidRequest
		}
		if anchor != "" {
			exists, err = corpus.AnchorExists(ctx, key, anchor)
			if err != nil {
				return Report{}, corpusFailure(err)
			}
			if !exists {
				return Report{}, ErrInvalidRequest
			}
		}
		report.DocumentKey, report.Anchor, report.DocumentNumber = key, anchor, doc.Number
	}
	for name, raw := range in.Suggestion {
		value, err := stringMember(raw)
		if err != nil {
			return Report{}, err
		}
		switch name {
		case "status_class":
			switch value {
			case "in_force", "partial", "expired", "not_yet", "suspended", "unknown":
				report.Suggestion.StatusClass = value
			default:
				return Report{}, ErrInvalidRequest
			}
		case "eff_from", "eff_to", "issue_date":
			if !validDate(value, name == "issue_date", now) {
				return Report{}, ErrInvalidRequest
			}
			switch name {
			case "eff_from":
				report.Suggestion.EffFrom = value
			case "eff_to":
				report.Suggestion.EffTo = value
			case "issue_date":
				report.Suggestion.IssueDate = value
			}
		case "document_number":
			number, err := validateNumber(ctx, corpus, in.Kind, value, now)
			if err != nil {
				return Report{}, err
			}
			report.Target, report.Suggestion.DocumentNumber = number, number
		}
	}
	s := report.Suggestion
	if s.EffFrom != "" && s.EffTo != "" && s.EffFrom > s.EffTo {
		return Report{}, ErrInvalidRequest
	}
	if ctx.Err() != nil {
		return Report{}, ErrUnavailable
	}
	return report, nil
}

func validateFields(in Input) error {
	if in.Country != "vn" {
		return ErrInvalidRequest
	}
	switch in.Kind {
	case "wrong_text", "wrong_validity", "broken_citation", "missing_document":
	default:
		return ErrInvalidRequest
	}
	if in.Handle == "" {
		if in.Kind != "missing_document" {
			return ErrInvalidRequest
		}
	} else {
		if !boundedString(in.Handle, retrieval.ReadHandleMax) {
			return ErrInvalidRequest
		}
		_, anchor, err := retrieval.ParseHandle(in.Handle)
		if err != nil || in.Kind == "missing_document" && anchor == "" {
			return ErrInvalidRequest
		}
	}
	for member := range in.Suggestion {
		allowed := false
		switch in.Kind {
		case "wrong_validity":
			allowed = member == "status_class" || member == "eff_from" || member == "eff_to"
		case "broken_citation":
			allowed = member == "document_number"
		case "missing_document":
			allowed = member == "document_number" || member == "issue_date"
		}
		if !allowed {
			return ErrInvalidRequest
		}
	}
	if in.Kind == "missing_document" {
		if _, present := in.Suggestion["document_number"]; !present {
			return ErrInvalidRequest
		}
	}
	return nil
}

func boundedString(value string, limit int) bool {
	return value != "" && utf8.ValidString(value) && !strings.ContainsRune(value, 0) && utf8.RuneCountInString(value) <= limit
}

func validDate(value string, issue bool, now time.Time) bool {
	date, err := time.Parse(time.DateOnly, value)
	if err != nil || date.Format(time.DateOnly) != value || value < "1945-09-02" {
		return false
	}
	today := now.UTC()
	upper := time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, time.UTC)
	if !issue {
		upper = upper.AddDate(10, 0, 0)
	}
	return !date.After(upper)
}
