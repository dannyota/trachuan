package cli

import (
	"bytes"
	"context"
	"testing"
)

func TestArticleOptions_RequiresLabeledPilotAndFamilyArtifact(t *testing.T) {
	valid := []string{"--families", "families.json", "--select", "labels:taxonomy-labels.json", "--recipe", "claude-haiku-5-5", "--out", "output", "--budget", "pilot", "--budget-usd", "15", "--dry-run"}
	o, err := parseArticleOptions("vn", "20990101", valid)
	if err != nil || o.Labels != "taxonomy-labels.json" || o.Families != "families.json" || !o.Options.DryRun {
		t.Fatalf("options %+v error %v", o, err)
	}
	for _, args := range [][]string{nil, valid[:8], {"--families", "f", "--select", "all", "--recipe", "claude-haiku-5-5", "--out", "o", "--budget", "pilot", "--budget-usd", "15"}, append(append([]string{}, valid...), "--unknown")} {
		if _, err = parseArticleOptions("vn", "20990101", args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	var out, errout bytes.Buffer
	if code := runClassify(context.Background(), []string{"articles", "vn", "20990101"}, &out, &errout); code != 2 {
		t.Fatalf("exit %d: %s", code, &errout)
	}
}

func TestScoreOptions_RequiresEveryArtifact(t *testing.T) {
	valid := []string{"--labels", "labels.json", "--families", "families.json", "--articles", "articles.json", "--out", "output"}
	o, err := parseScoreOptions("vn", "20990101", valid)
	if err != nil || o.Labels != "labels.json" || o.Out != "output" {
		t.Fatalf("options %+v error %v", o, err)
	}
	if _, err = parseScoreOptions("vn", "20990101", valid[:6]); err == nil {
		t.Fatal("accepted missing output")
	}
	var out, errout bytes.Buffer
	if code := runClassify(context.Background(), []string{"score", "vn", "20990101"}, &out, &errout); code != 2 {
		t.Fatalf("exit %d: %s", code, &errout)
	}
}
