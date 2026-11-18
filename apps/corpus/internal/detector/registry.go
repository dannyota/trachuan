package detector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type Severity string

const (
	Blocking Severity = "blocking"
	Review   Severity = "review"
	Info     Severity = "info"
)

const (
	SubjectStageItem       = "stage_item"
	DiscriminatorGapDetail = "gap_detail"
	CompletenessStageItem  = "stage_item_completed"
)

var ErrUnregisteredGap = errors.New("gap kind is not registered")

type Detector struct {
	ID            string   `json:"id"`
	Version       int      `json:"version"`
	Semantics     int      `json:"semantics"`
	Subject       string   `json:"subject"`
	Inputs        []string `json:"inputs"`
	Severity      Severity `json:"severity"`
	Fingerprint   []string `json:"fingerprint"`
	Discriminator string   `json:"discriminator"`
	Completeness  string   `json:"completeness"`
}

var registry = newRegistry()

func newRegistry() []Detector {
	kinds := gapKinds()
	slices.Sort(kinds)
	detectors := make([]Detector, 0, len(kinds))
	for _, kind := range kinds {
		detectors = append(detectors, Detector{
			ID: kind, Version: 1, Semantics: 1, Subject: SubjectStageItem,
			Inputs: []string{"stage_item_outcome"}, Severity: Review,
			Fingerprint:   []string{"stage", "item_key", "kind", "detail"},
			Discriminator: DiscriminatorGapDetail, Completeness: CompletenessStageItem,
		})
	}
	return detectors
}

func compareByID(a, b Detector) int { return strings.Compare(a.ID, b.ID) }

func compareToID(d Detector, id string) int { return strings.Compare(d.ID, id) }

func (d Detector) clone() Detector {
	d.Inputs = slices.Clone(d.Inputs)
	d.Fingerprint = slices.Clone(d.Fingerprint)
	return d
}

func All() []Detector {
	out := make([]Detector, len(registry))
	for i, d := range registry {
		out[i] = d.clone()
	}
	return out
}

func Lookup(id string) (Detector, bool) {
	i, ok := slices.BinarySearchFunc(registry, id, compareToID)
	if !ok {
		return Detector{}, false
	}
	return registry[i].clone(), true
}

func ValidateKind(kind string) error {
	if _, ok := slices.BinarySearchFunc(registry, kind, compareToID); ok {
		return nil
	}
	return fmt.Errorf("%w: kind %q", ErrUnregisteredGap, kind)
}

func canonical(detectors []Detector) []byte {
	sorted := make([]Detector, len(detectors))
	for i, d := range detectors {
		d = d.clone()
		sorted[i] = d
	}
	slices.SortFunc(sorted, compareByID)
	data, err := json.Marshal(struct {
		Detectors []Detector `json:"detectors"`
	}{sorted})
	if err != nil {
		panic(fmt.Sprintf("marshal detector registry: %v", err))
	}
	return data
}

func hashOf(detectors []Detector) string {
	sum := sha256.Sum256(canonical(detectors))
	return hex.EncodeToString(sum[:])
}

func Canonical() []byte { return canonical(registry) }

func Hash() string { return hashOf(registry) }
