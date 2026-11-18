// Package stage defines shared pipeline stage types.
package stage

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Snapshot holds the frozen inputs for a run.
type Snapshot struct {
	SourceList                                       json.RawMessage
	SourceListHash                                   string
	Stages                                           []string
	Recipes                                          map[string]string
	TaxonomyVersion, WorkerVersion, PreviousManifest string
	PreviousReport                                   *PreviousReport `json:",omitempty"`
}

const EvidenceStage = "evidence"

const NoPreviousReport = "none"

const PreviousReportFile = "report.json"

type PreviousReport struct {
	SHA256 string `json:"sha256"`
	Path   string `json:"path,omitempty"`
}

func (p PreviousReport) None() bool { return p.SHA256 == NoPreviousReport }

var ErrNoDecision = errors.New("run has no previous report decision")

func LegacyCutoff(country string) string {
	if country == "vn" {
		return "20261118_2"
	}
	return ""
}

func PredatesDecision(country, rel string, s Snapshot) bool {
	cutoff := LegacyCutoff(country)
	return s.PreviousReport == nil && cutoff != "" && CompareRel(rel, cutoff) <= 0
}

func RequireDecision(country, rel string, s Snapshot) error {
	if s.PreviousReport != nil || PredatesDecision(country, rel, s) {
		return nil
	}
	return fmt.Errorf("%w: %s/%s", ErrNoDecision, country, rel)
}

func EvaluatedAfter(country, rel string, s Snapshot, name string) bool {
	if PredatesDecision(country, rel, s) {
		return name == "eval"
	}
	return name == EvidenceStage
}

func CompareRel(a, b string) int {
	ap := strings.SplitN(a, "_", 2)
	bp := strings.SplitN(b, "_", 2)
	if ap[0] != bp[0] {
		return strings.Compare(ap[0], bp[0])
	}
	var ai, bi int
	if len(ap) == 2 {
		ai, _ = strconv.Atoi(ap[1])
	}
	if len(bp) == 2 {
		bi, _ = strconv.Atoi(bp[1])
	}
	if ai < bi {
		return -1
	}
	if ai > bi {
		return 1
	}
	return 0
}

// Run identifies a pipeline run and its frozen snapshot.
type Run struct {
	ID           int64
	Country, Rel string
	Generation   int
	Snapshot     Snapshot
}

// Gap describes an incomplete item or missing evidence.
type Gap struct {
	Kind, Detail string
	Source       string
	Origin       Origin
	Cause        Cause
	Endpoint     string
	HTTPStatus   *int32
	EvidenceRef  string
}

// Origin identifies the source of a failure.
type Origin string

// Supported failure origins.
const (
	OriginGateway    Origin = "gateway"
	OriginSourceData Origin = "source_data"
	OriginTransport  Origin = "transport"
	OriginUnknown    Origin = "unknown"
)

// Cause identifies a structured failure cause.
type Cause string

// Supported failure causes.
const (
	CauseHTTP5xx         Cause = "http_5xx"
	CauseHTTP429         Cause = "http_429"
	CauseNonJSON200      Cause = "non_json_200"
	CauseTimeout         Cause = "timeout"
	CauseConnectionError Cause = "connection_error"
	CauseInvalidPayload  Cause = "invalid_payload"
	CauseUnknown         Cause = "unknown"
)

// GapCause holds the optional structured fields of a failure.
type GapCause struct {
	Source      string
	Origin      Origin
	Cause       Cause
	Endpoint    string
	HTTPStatus  *int32
	EvidenceRef string
}

// Validate rejects unknown structured enumerations.
func (g Gap) Validate() error {
	switch g.Origin {
	case "", OriginGateway, OriginSourceData, OriginTransport, OriginUnknown:
	default:
		return fmt.Errorf("invalid gap origin %q", g.Origin)
	}
	switch g.Cause {
	case "", CauseHTTP5xx, CauseHTTP429, CauseNonJSON200, CauseTimeout, CauseConnectionError, CauseInvalidPayload, CauseUnknown:
	default:
		return fmt.Errorf("invalid gap cause %q", g.Cause)
	}
	return nil
}

// RetryEligible reports whether a gap qualifies for restart retry.
func RetryEligible(g Gap) bool {
	return g.Kind == "source_unavailable" || (g.Kind == "invalid_source_data" && g.Origin == OriginGateway && (g.Cause == CauseNonJSON200 || g.Cause == CauseHTTP5xx))
}

func (g *Gap) setCause(c GapCause) {
	g.Source, g.Origin, g.Cause, g.Endpoint, g.HTTPStatus, g.EvidenceRef = c.Source, c.Origin, c.Cause, c.Endpoint, c.HTTPStatus, c.EvidenceRef
}

// Item identifies stage work and an optional reason to skip it.
type Item struct {
	Key, InputHash string
	Skip           *Gap
}

// Counts summarizes stage progress and outstanding gaps.
type Counts struct {
	Expected, Done, Gap, Failed, Pending int
	UnacceptedGaps                       map[string]int
}

// RecipeHash hashes the NUL-separated recipe components.
func RecipeHash(stage, version string, extra ...string) string {
	parts := append([]string{stage, version}, extra...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}
