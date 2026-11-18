package evidence

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/intake"
)

const reportsFile = "reports.yaml"

type reportDetector struct{}

func (reportDetector) Spec() detector.Detector { return specOf(detector.ExternalReport) }

func (reportDetector) Detect(_ context.Context, in Inputs) (Observation, error) {
	dir, sum := in.BuildDir, in.BuildSHA256
	if dir == "" && in.Build != nil {
		var err error
		if dir, sum, err = in.Build(); err != nil {
			return Observation{}, err
		}
	}
	if dir == "" || sum == "" {
		return Observation{}, fmt.Errorf("build artifact is not available")
	}
	return detectReports(in.Run.Country, dir, sum)
}

func detectReports(country, buildDir, buildSHA256 string) (Observation, error) {
	complete := func(Subject) bool { return true }
	manifest, err := os.ReadFile(filepath.Join(buildDir, manifestFile))
	if err != nil {
		return Observation{}, fmt.Errorf("read build manifest: %w", err)
	}
	if bareSHA256(manifest) != buildSHA256 {
		return Observation{}, fmt.Errorf("build artifact failed verification")
	}
	path := filepath.Join(buildDir, reportsFile)
	if !manifestLists(string(manifest), reportsFile) {
		if _, err = os.Lstat(path); err == nil {
			return Observation{}, fmt.Errorf("build artifact holds a reports file its manifest does not list")
		} else if !os.IsNotExist(err) {
			return Observation{}, fmt.Errorf("inspect reports: %w", err)
		}
		return Observation{ObservedCompletely: complete}, nil
	}
	if _, err = artifact.VerifyFiles(buildDir, reportsFile); err != nil {
		return Observation{}, fmt.Errorf("reports file failed verification: %w", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Observation{}, fmt.Errorf("read reports: %w", err)
	}
	file, err := intake.Parse(country, data)
	if err != nil {
		return Observation{}, err
	}
	findings := make([]Fired, 0, file.Len())
	for _, r := range file.Records {
		target := r.Target.Key + "\x1f" + r.Target.Anchor
		if r.Target.Number != "" {
			target = "number\x1f" + r.Target.Number
		}
		claim := r.Claim.Code + "\x1f" + r.Claim.Acting.Key + "\x1f" + r.Claim.Acting.Anchor
		findings = append(findings, Fired{
			Subject:           Subject{Type: detector.SubjectReport, Fields: []string{r.Origin, r.OriginID}},
			Discriminator:     detector.DiscriminatorNone,
			FingerprintInputs: []string{r.Kind, target, claim},
		})
	}
	return Observation{Findings: findings, Observed: len(findings), ObservedCompletely: complete}, nil
}
