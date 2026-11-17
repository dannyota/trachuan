package cli

import (
	"strings"
	"testing"
)

func TestArticleOptions_TwoSteps(t *testing.T) {
	base := []string{"--families", "families.json", "--select", "labels:labels.json", "--recipe", "claude-haiku-5-5", "--out", "output", "--budget", "pilot", "--budget-usd", "15", "--dry-run"}
	for _, step := range []string{"1", "2"} {
		args := append(append([]string(nil), base...), "--step", step)
		if step == "2" {
			args = append(args, "--step-1", "step1/articles.json")
		}
		o, err := parseArticleOptions("vn", "20990101", args)
		if err != nil {
			t.Fatal(err)
		}
		if !o.Options.DryRun {
			t.Fatal("lost dry run")
		}
	}
	for _, suffix := range [][]string{{"--step", "0"}, {"--step", "3"}, {"--step", "x"}, {"--step", "2"}, {"--step-1", "step1/articles.json"}, {"--step", "1", "--step-1", "step1/articles.json"}} {
		if _, err := parseArticleOptions("vn", "20990101", append(append([]string(nil), base...), suffix...)); err == nil {
			t.Fatalf("accepted %v", suffix)
		}
	}
	sonnet := append([]string(nil), base...)
	sonnet[5] = "claude-sonnet-5-5"
	if _, err := parseArticleOptions("vn", "20990101", append(sonnet, "--step", "1")); err == nil || !strings.Contains(err.Error(), "Haiku") {
		t.Fatalf("Sonnet error %v", err)
	}
}
