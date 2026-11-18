package knowledge

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	RulesVersion    = "knowledge/1"
	ContractVersion = 1
	MethodReviewed  = "reviewed"
	MaxFileBytes    = 8 << 20
	MaxEntries      = 20000
)

const (
	KindDecision = "review_decision"
	KindEffect   = "provision_effect"
	KindRelation = "relation"
)

type Reviewed struct {
	Date, By, Method, Batch string
	SampleRead, SampleOf    int
}

type Ancestor struct {
	Anchor        string `yaml:"anchor"`
	HeadingSHA256 string `yaml:"heading_sha256"`
}

type ProvisionDepends struct {
	NumberNorm    string     `yaml:"number_norm"`
	TextSource    string     `yaml:"text_source"`
	TextSHA256    string     `yaml:"text_sha256"`
	HeadingSHA256 string     `yaml:"heading_sha256"`
	Ancestors     []Ancestor `yaml:"ancestors"`
}

func (d ProvisionDepends) YAML() ([]byte, error) {
	if d.Ancestors == nil {
		d.Ancestors = []Ancestor{}
	}
	return yaml.Marshal(d)
}

type ReviewDecision struct {
	ID          string
	Position    int
	Finding     string
	ReasonCode  string
	Reason      string
	Fingerprint string
	Semantics   int
	FirstRel    string
	Reviewed    Reviewed
}

type Claim struct {
	ID               string
	Kind             string
	Position         int
	ActingKey        string
	ActingAnchor     string
	TargetKey        string
	TargetAnchor     string
	RelationKind     string
	Quote            string
	Contract         int
	ActingDepends    ProvisionDepends
	TargetDepends    ProvisionDepends
	TargetNumberNorm string
	Reviewed         Reviewed
}

type File struct {
	Decisions []ReviewDecision
	Claims    []Claim
}

func (f File) Len() int { return len(f.Decisions) + len(f.Claims) }

type ValidationError struct{ Problems []string }

func (e *ValidationError) Error() string {
	const shown = 20
	problems := e.Problems
	suffix := ""
	if len(problems) > shown {
		suffix = "; and more"
		problems = problems[:shown]
	}
	return "invalid knowledge: " + strings.Join(problems, "; ") + suffix
}

func Hash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}
