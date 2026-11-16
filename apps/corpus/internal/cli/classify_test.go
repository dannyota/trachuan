package cli

import (
	"bytes"
	"context"
	"testing"
)

func TestClassify_RequiresExplicitPilotBudgetAndDraw(t *testing.T) {
	valid := []string{"--documents", "20", "--seed", "42", "--recipe", "claude-haiku-5-5", "--out", "synthetic-output", "--budget", "pilot", "--budget-usd", "15", "--dry-run"}
	o, err := parseScreenOptions("vn", "20990101", valid)
	if err != nil || o.Documents != 20 || o.Seed != 42 || o.BudgetMicroUSD != 15000000 || !o.DryRun {
		t.Fatalf("options %+v error %v", o, err)
	}
	for _, args := range [][]string{nil, valid[:10], append(append([]string(nil), valid...), "--budget-usd", "15"), {"--documents", "401", "--seed", "1", "--recipe", "claude-haiku-5-5", "--out", "synthetic-output", "--budget", "pilot", "--budget-usd", "15"}, {"--documents", "1", "--seed", "1", "--recipe", "claude-haiku-5-5", "--out", "synthetic-output", "--budget", "full", "--budget-usd", "100"}} {
		if _, err := parseScreenOptions("vn", "20990101", args); err == nil {
			t.Fatalf("accepted invalid options %v", args)
		}
	}
	var out, errout bytes.Buffer
	if code := Run(context.Background(), []string{"classify", "screen", "vn", "20990101"}, &out, &errout); code != 2 || errout.Len() == 0 {
		t.Fatalf("code %d error %s", code, &errout)
	}
}
