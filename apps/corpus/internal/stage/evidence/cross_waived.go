package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/detector"
)

type artifactDirs func(stage, itemKey string) (string, bool, error)

type waivedDetector struct{}

func (waivedDetector) Spec() detector.Detector {
	return specOf(detector.WaivedDocumentWithoutRelations)
}

func (waivedDetector) Detect(ctx context.Context, in Inputs) (Observation, error) {
	return detectWaived(ctx, sqlRelease{in.Release}, in.ArtifactDir)
}

func detectWaived(ctx context.Context, rel releaseData, dirs artifactDirs) (Observation, error) {
	gaps, err := rel.ExtractGaps(ctx)
	if err != nil {
		return Observation{}, err
	}
	observed := map[string]bool{}
	var findings []Fired
	obs := Observation{}
	err = rel.Documents(ctx, func(d releaseDocument) error {
		ocr := d.TextSource == "ocr"
		if d.Baseline || neverRead(d) {
			if ocr {
				obs.NotObservable++
			}
			return nil
		}
		if !ocr {
			observed[d.Key] = d.TextSource != "none"
			return nil
		}
		check, waived, checkErr := waiverCheck(d, dirs)
		if checkErr != nil {
			return checkErr
		}
		if waived && !d.TextRelation {
			findings = append(findings, documentFired(d.Key, "", check, d.ExtractSHA256))
		}
		if lostInput(gaps[d.Key]) {
			obs.NotObservable++
			return nil
		}
		obs.Observed++
		observed[d.Key] = true
		return nil
	})
	if err != nil {
		return Observation{}, err
	}
	obs.Findings = findings
	obs.ObservedCompletely = func(s Subject) bool { return len(s.Fields) > 0 && observed[s.Fields[0]] }
	return obs, nil
}

func lostInput(gaps map[string]bool) bool {
	for kind := range gaps {
		if kind != "ocr_flagged" {
			return true
		}
	}
	return false
}

func waiverCheck(d releaseDocument, dirs artifactDirs) (string, bool, error) {
	if dirs == nil {
		return "", false, errors.New("extract artifacts are not available")
	}
	dir, ok, err := dirs("extract", d.Key)
	if err != nil {
		return "", false, err
	}
	if !ok {
		return "", false, fmt.Errorf("document %s has no done extract item", d.Key)
	}
	sum, err := artifact.Verify(dir)
	if err != nil || sum != d.ExtractSHA256 {
		return "", false, fmt.Errorf("extract artifact of %s does not match the release", d.Key)
	}
	data, err := os.ReadFile(filepath.Join(dir, "text.json"))
	if err != nil {
		return "", false, fmt.Errorf("read extract text of %s: %w", d.Key, err)
	}
	var record struct {
		Waiver *struct {
			CheckSHA256 string `json:"check_sha256"`
		} `json:"waiver"`
	}
	if err = json.Unmarshal(data, &record); err != nil {
		return "", false, fmt.Errorf("decode extract text of %s: %w", d.Key, err)
	}
	if record.Waiver == nil {
		return "", false, nil
	}
	return record.Waiver.CheckSHA256, true, nil
}
