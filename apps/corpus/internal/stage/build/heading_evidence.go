package build

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
)

const headingProjection = 1

func recordHeadingEvidence(record map[string]any, scanHash, exclusionsHash string) {
	record["heading_projection"] = headingProjection
	record["heading_scan_sha256"] = scanHash
	record["heading_exclusions_sha256"] = exclusionsHash
}

func WriteHeadingScan(dir string, data []byte) (string, error) {
	hash := hashBytes(data)
	for _, f := range []struct {
		name string
		data []byte
	}{{"heading-scan.json", data}, {"heading-scan.sha256", []byte(hash + "  heading-scan.json\n")}} {
		path := filepath.Join(dir, f.name)
		existing, err := os.ReadFile(path)
		if err == nil {
			if !bytes.Equal(existing, f.data) {
				return "", fmt.Errorf("stored %s differs from candidate scan", f.name)
			}
			continue
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("read %s: %w", f.name, err)
		}
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return "", fmt.Errorf("create %s: %w", f.name, err)
		}
		_, writeErr := file.Write(f.data)
		closeErr := file.Close()
		if writeErr != nil {
			return "", fmt.Errorf("write %s: %w", f.name, writeErr)
		}
		if closeErr != nil {
			return "", fmt.Errorf("close %s: %w", f.name, closeErr)
		}
	}
	return hash, nil
}

func verifyHeadingEvidence(dir string, record BuildRecord) error {
	for _, f := range []struct{ name, hash string }{{"heading-exclusions.json", record.HeadingExclusionsSHA256}, {"heading-scan.json", record.HeadingScanSHA256}} {
		b, err := os.ReadFile(filepath.Join(dir, f.name))
		if err != nil {
			return fmt.Errorf("read %s: %w", f.name, err)
		}
		if hashBytes(b) != f.hash {
			return fmt.Errorf("%s hash mismatch", f.name)
		}
	}
	return nil
}

func (input HeadingExclusionsInput) unchanged(hash string) error {
	_, actual, err := input.pinned()
	if err != nil {
		return err
	}
	if actual != hash {
		return fmt.Errorf("heading exclusions changed during build")
	}
	return nil
}
