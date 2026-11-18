package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/detector"
)

const (
	StatusOpen     = "open"
	StatusAccepted = "accepted"

	schemaVersion = 1
	findingsFile  = "findings.json"
	hashPrefix    = "sha256:"
	noneValue     = "none"
)

type Bindings struct {
	Rel                       string `json:"rel"`
	DumpSHA256                string `json:"dump_sha256"`
	BuildArtifactSHA256       string `json:"build_artifact_sha256"`
	ReportSHA256              string `json:"report_sha256"`
	DetectorRegistrySHA256    string `json:"detector_registry_sha256"`
	PredecessorRel            string `json:"predecessor_rel"`
	PredecessorFindingsSHA256 string `json:"predecessor_findings_sha256"`
}

type DetectorCounts struct {
	Complete      bool `json:"complete"`
	Observed      int  `json:"observed"`
	NotObservable int  `json:"not_observable"`
	Excluded      int  `json:"excluded"`
}

type Finding struct {
	Key           string            `json:"key"`
	Detector      string            `json:"detector"`
	Subject       Subject           `json:"subject"`
	Discriminator string            `json:"discriminator"`
	Fingerprint   string            `json:"fingerprint"`
	Semantics     int               `json:"semantics"`
	Severity      detector.Severity `json:"severity"`
	FirstRel      string            `json:"first_rel"`
	Status        string            `json:"status"`
	Carried       bool              `json:"carried"`
}

type ReviewDecision struct {
	ID          string `json:"id"`
	Key         string `json:"key"`
	Fingerprint string `json:"fingerprint"`
	Semantics   int    `json:"semantics"`
	FirstRel    string `json:"first_rel"`
}

type File struct {
	Schema    int                       `json:"schema"`
	Bindings  Bindings                  `json:"bindings"`
	Detectors map[string]DetectorCounts `json:"detectors"`
	Findings  []Finding                 `json:"findings"`
	New       []string                  `json:"new"`
	Gone      []string                  `json:"gone"`
	Carried   []string                  `json:"carried"`
	Unmatched []ReviewDecision          `json:"unmatched"`
}

func (f File) encode() ([]byte, error) {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode findings: %w", err)
	}
	return append(data, '\n'), nil
}

type detectorRun struct {
	spec        detector.Detector
	observation Observation
}

type computeInput struct {
	Rel       string
	Bindings  Bindings
	Runs      []detectorRun
	Previous  *File
	Decisions []ReviewDecision
}

func hashValue(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode hash input: %w", err)
	}
	sum := sha256.Sum256(data)
	return hashPrefix + hex.EncodeToString(sum[:]), nil
}

func findingKey(id string, f Fired) (string, error) {
	return hashValue([]any{id, f.Subject.Type, f.Subject.Fields, f.Discriminator})
}

func compute(in computeInput) (File, error) {
	file := File{Schema: schemaVersion, Bindings: in.Bindings, Detectors: map[string]DetectorCounts{}, Findings: []Finding{}, New: []string{}, Gone: []string{}, Carried: []string{}, Unmatched: []ReviewDecision{}}
	previous := map[string]Finding{}
	var prior []Finding
	if in.Previous != nil {
		prior = in.Previous.Findings
		for _, f := range prior {
			previous[f.Key] = f
		}
	}
	fired := map[string]bool{}
	observed := map[string]func(Subject) bool{}
	for _, run := range in.Runs {
		id := run.spec.ID
		file.Detectors[id] = DetectorCounts{Complete: true, Observed: run.observation.Observed, NotObservable: run.observation.NotObservable, Excluded: run.observation.Excluded}
		observed[id] = run.observation.ObservedCompletely
		for _, f := range run.observation.Findings {
			finding, err := firedFinding(run.spec, f, in.Rel)
			if err != nil {
				return File{}, &IncompleteError{Detector: id, Reason: err.Error()}
			}
			if fired[finding.Key] {
				return File{}, &IncompleteError{Detector: id, Reason: "duplicate finding " + finding.Key}
			}
			fired[finding.Key] = true
			if p, ok := previous[finding.Key]; ok && p.Fingerprint == finding.Fingerprint && p.Semantics == finding.Semantics {
				finding.FirstRel = p.FirstRel
			}
			if finding.FirstRel == in.Rel {
				file.New = append(file.New, finding.Key)
			}
			file.Findings = append(file.Findings, finding)
		}
	}
	for _, p := range prior {
		if fired[p.Key] {
			continue
		}
		if seen := observed[p.Detector]; seen != nil && seen(p.Subject) {
			file.Gone = append(file.Gone, p.Key)
			continue
		}
		p.Carried, p.Status = true, StatusOpen
		file.Carried = append(file.Carried, p.Key)
		file.Findings = append(file.Findings, p)
	}
	applyDecisions(&file, in.Decisions)
	slices.SortFunc(file.Findings, func(a, b Finding) int { return strings.Compare(a.Key, b.Key) })
	slices.Sort(file.New)
	slices.Sort(file.Gone)
	slices.Sort(file.Carried)
	return file, nil
}

func firedFinding(spec detector.Detector, f Fired, rel string) (Finding, error) {
	id, semantics := spec.ID, spec.Semantics
	severity, err := findingSeverity(spec.Severity, f.Severity)
	if err != nil {
		return Finding{}, err
	}
	if err = f.Subject.validate(); err != nil {
		return Finding{}, err
	}
	key, err := findingKey(id, f)
	if err != nil {
		return Finding{}, err
	}
	fingerprint, err := hashValue(f.FingerprintInputs)
	if err != nil {
		return Finding{}, err
	}
	return Finding{Key: key, Detector: id, Subject: f.Subject, Discriminator: f.Discriminator, Fingerprint: fingerprint, Semantics: semantics, Severity: severity, FirstRel: rel, Status: StatusOpen}, nil
}

func applyDecisions(file *File, decisions []ReviewDecision) {
	matched := make([]bool, len(decisions))
	for i := range file.Findings {
		f := &file.Findings[i]
		for j, d := range decisions {
			if d.Key == f.Key && d.Fingerprint == f.Fingerprint && d.Semantics == f.Semantics && d.FirstRel == f.FirstRel {
				f.Status = StatusAccepted
				matched[j] = true
			}
		}
	}
	for j, d := range decisions {
		if !matched[j] {
			file.Unmatched = append(file.Unmatched, d)
		}
	}
	slices.SortFunc(file.Unmatched, func(a, b ReviewDecision) int {
		if c := strings.Compare(a.ID, b.ID); c != 0 {
			return c
		}
		return strings.Compare(a.Key, b.Key)
	})
}

func severityRank(s detector.Severity) int {
	switch s {
	case detector.Info:
		return 1
	case detector.Review:
		return 2
	case detector.Blocking:
		return 3
	}
	return 0
}

func findingSeverity(registered, fired detector.Severity) (detector.Severity, error) {
	if fired == "" {
		return registered, nil
	}
	if (fired != detector.Review && fired != detector.Info) || severityRank(fired) > severityRank(registered) {
		return "", fmt.Errorf("severity %q is not allowed for a %s detector", fired, registered)
	}
	return fired, nil
}
