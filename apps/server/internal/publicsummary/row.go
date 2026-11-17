package publicsummary

import (
	"encoding/json"
	"time"
)

type failure string

func (f failure) Error() string { return string(f) }

const (
	ErrBusy        failure = "public summaries busy"
	ErrUnavailable failure = "public summaries unavailable"
	ErrNotFound    failure = "public summary not found"
	ErrTooLarge    failure = "public summaries too large"
)

type Release struct {
	ActivatedAt      time.Time
	Database         string
	ID               string
	Projection       int
	HeadingsApproved bool
}

type Row struct {
	Key              string
	Indexed          bool
	StatusClass      string
	StatusBasis      string
	IsCurrent        bool
	ValidityConflict bool
	EffectiveFrom    *time.Time
	EffectiveTo      *time.Time
	GapKinds         []string
	Payload          json.RawMessage
}

type Item struct {
	Key   string `json:"key"`
	Index bool   `json:"index"`
}
