// Package reports admits structured corpus defect reports.
package reports

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"time"
	"unicode/utf8"

	"danny.vn/trachuan/apps/server/internal/apierror"
)

// Error is a fixed public report failure code.
type Error string

// Error returns the fixed public message without request or database data.
func (e Error) Error() string { return apierror.Message(string(e)) }

const (
	// ErrInvalidRequest means a field failed structured validation.
	ErrInvalidRequest Error = "invalid_request"
	// ErrRateLimited means an admission cap was reached.
	ErrRateLimited Error = "rate_limited"
	// ErrReleaseExpired means the requested release is no longer active.
	ErrReleaseExpired Error = "release_expired"
	// ErrUnavailable means admission could not be confirmed.
	ErrUnavailable Error = "unavailable"
	// ErrUnauthenticated matches the bearer check for blocked accounts.
	ErrUnauthenticated Error = "unauthenticated"
)

// Input accepts only the structured report fields.
type Input struct {
	Country    string                     `json:"country"`
	Kind       string                     `json:"kind"`
	Handle     string                     `json:"handle,omitempty"`
	Release    string                     `json:"release,omitempty"`
	Suggestion map[string]json.RawMessage `json:"suggestion,omitempty"`
}

// UnmarshalJSON rejects unknown, duplicate, null and wrongly typed members.
// Both transports must decode this type rather than a permissive alias.
func (in *Input) UnmarshalJSON(data []byte) error {
	fields, err := objectMembers(data)
	if err != nil {
		return err
	}
	var next Input
	for name, raw := range fields {
		var dst *string
		switch name {
		case "country":
			dst = &next.Country
		case "kind":
			dst = &next.Kind
		case "handle":
			dst = &next.Handle
		case "release":
			dst = &next.Release
		case "suggestion":
			next.Suggestion, err = objectMembers(raw)
			if err != nil {
				return err
			}
			continue
		default:
			return ErrInvalidRequest
		}
		value, err := stringMember(raw)
		if err != nil {
			return err
		}
		*dst = value
	}
	*in = next
	return nil
}

func objectMembers(data []byte) (map[string]json.RawMessage, error) {
	if !utf8.Valid(data) {
		return nil, ErrInvalidRequest
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil || start != json.Delim('{') {
		return nil, ErrInvalidRequest
	}
	fields := make(map[string]json.RawMessage)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, ErrInvalidRequest
		}
		name, ok := token.(string)
		if !ok {
			return nil, ErrInvalidRequest
		}
		if _, duplicate := fields[name]; duplicate {
			return nil, ErrInvalidRequest
		}
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return nil, ErrInvalidRequest
		}
		fields[name] = raw
	}
	if end, err := decoder.Token(); err != nil || end != json.Delim('}') {
		return nil, ErrInvalidRequest
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, ErrInvalidRequest
	}
	return fields, nil
}

func stringMember(raw json.RawMessage) (string, error) {
	raw = bytes.TrimSpace(raw)
	var value string
	if len(raw) == 0 || raw[0] != '"' || !utf8.Valid(raw) || json.Unmarshal(raw, &value) != nil {
		return "", ErrInvalidRequest
	}
	return value, nil
}

// Result identifies an admitted or existing open report.
type Result struct {
	ReportID     int64  `json:"report_id"`
	State        string `json:"state"`
	Deduplicated bool   `json:"deduplicated"`
}

// Suggestion holds only validated, canonical values.
type Suggestion struct {
	StatusClass    string `json:"status_class,omitempty"`
	EffFrom        string `json:"eff_from,omitempty"`
	EffTo          string `json:"eff_to,omitempty"`
	DocumentNumber string `json:"document_number,omitempty"`
	IssueDate      string `json:"issue_date,omitempty"`
}

// GroupKey is stable across corpus releases.
type GroupKey struct {
	DocumentKey string
	Anchor      string
	Kind        string
	Target      string
}

// Report keeps the corpus identity and typed suggestion checked at admission.
type Report struct {
	GroupKey
	Release        string
	DocumentNumber string
	Suggestion     Suggestion
}

// Corpus exposes immutable identity lookups on a leased release.
type Corpus interface {
	Document(context.Context, string) (Document, bool, error)
	AnchorExists(context.Context, string, string) (bool, error)
	HasNumber(context.Context, string) (bool, error)
	// HasNumberSegment tests the release's cached normalized number segments.
	HasNumberSegment(context.Context, string) (bool, error)
}

// Document supplies the original corpus number for durable report identity.
type Document struct{ Number string }

// Lease holds a release open until Done is called after validation and admission.
type Lease struct {
	Corpus  Corpus
	Release string
	Done    func()
}

// Resolver lends the active release, rejecting a nonactive requested release.
// It may return ErrReleaseExpired or release.ErrExpired for a retired release.
type Resolver interface {
	Resolve(context.Context, string, string) (Lease, error)
}

// LockAccountForReportParams identifies the account to lock.
type LockAccountForReportParams struct{ AccountID int64 }

// AccountState reflects the locked account; missing accounts have Exists false.
type AccountState struct{ Exists, Blocked, DeletionRequested bool }

// CountAccountReportsParams counts every retained row and rows created since Since.
type CountAccountReportsParams struct {
	AccountID int64
	Since     time.Time
}

// AccountCounts includes closed and withdrawn rows in Daily and Retained.
type AccountCounts struct{ Daily, Open, Retained int64 }

// FindOpenReportParams scopes a duplicate lookup to its account and stable key.
type FindOpenReportParams struct {
	AccountID int64
	GroupKey
}

// InsertReportParams contains only canonical report values and the admission time.
type InsertReportParams struct {
	AccountID int64
	Report    Report
	CreatedAt time.Time
}

// Store is the transaction-bound report query view. Retained counts include
// export snapshot rows in the global count. FindOpenReport returns found false
// for no row; missing accounts return AccountState with Exists false.
type Store interface {
	LockAccountForReport(context.Context, LockAccountForReportParams) (AccountState, error)
	LockReports(context.Context) error
	CountAccountReports(context.Context, CountAccountReportsParams) (AccountCounts, error)
	CountRetainedReports(context.Context) (int64, error)
	FindOpenReport(context.Context, FindOpenReportParams) (Result, bool, error)
	InsertReport(context.Context, InsertReportParams) (Result, error)
}

// Transaction binds every query and its completion to the same transaction.
type Transaction interface {
	Store
	Commit(context.Context) error
	Rollback(context.Context) error
}

// TxRunner begins an admission transaction. The adapter supplies pgx behavior.
type TxRunner interface {
	Begin(context.Context) (Transaction, error)
}
