//go:build integration

package build

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBuild_ReportsFileInArtifactAndVerified(t *testing.T) {
	ctx := context.Background()
	input := reportsInputFile(t, reportsRecord)
	f := recoverySetupWith(t, func(cfg *Config) { cfg.Reports = input })
	dir := f.env.Artifacts.Abs(f.art.Path)
	data, err := os.ReadFile(filepath.Join(dir, reportsFileName))
	if err != nil || string(data) != reportsRecord {
		t.Fatalf("reports.yaml = %q, %v", data, err)
	}
	if _, err = New(f.cfg).Resolve(ctx, f.env, f.run, f.item, f.art); err != nil {
		t.Fatal(err)
	}
}

func TestBuild_NoReportsFileWithoutRecords(t *testing.T) {
	f := recoverySetup(t)
	if _, err := os.Stat(filepath.Join(f.env.Artifacts.Abs(f.art.Path), reportsFileName)); !os.IsNotExist(err) {
		t.Fatalf("reports.yaml: %v", err)
	}
}
