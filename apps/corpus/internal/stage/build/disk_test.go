package build

import (
	"math"
	"path/filepath"
	"testing"
)

func TestCheckDisk_RejectsInsufficient(t *testing.T) {
	if err := checkDisk(t.TempDir(), math.MaxUint64); err == nil {
		t.Fatal("accepted insufficient disk")
	}
}

func TestCheckDisks_RejectsMissingDatabasePath(t *testing.T) {
	cfg := Config{DatabaseDiskPath: filepath.Join(t.TempDir(), "missing"), diskCheck: func(string, uint64) error { return nil }}
	if err := cfg.checkDisks(t.TempDir(), 0); err == nil {
		t.Fatal("accepted missing database disk path")
	}
}

func TestDiskLimits_DefaultAndConfigured(t *testing.T) {
	cfg := Config{}
	if cfg.beforeBytes() != 30<<30 || cfg.duringBytes() != 10<<30 {
		t.Fatal("wrong defaults")
	}
	cfg.MinFreeBeforeBytes, cfg.MinFreeDuringBytes = 123, 45
	if cfg.beforeBytes() != 123 || cfg.duringBytes() != 45 {
		t.Fatal("configured thresholds ignored")
	}
}

func TestCheckDisks_RequiresDatabasePath(t *testing.T) {
	if err := (Config{}).checkDisks(t.TempDir(), 1); err == nil {
		t.Fatal("accepted an empty database disk path")
	}
}
