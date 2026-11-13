package pinned

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"danny.vn/trachuan/internal/retrieval"
)

type Reader interface {
	GetPinnedArtifact(context.Context, store.GetPinnedArtifactParams) (store.GetPinnedArtifactRow, error)
}

func Verify(ctx context.Context, q Reader, artifacts artifact.Store, country string, pins []source.Pin) error {
	keys, numbers := map[string]bool{}, map[string]bool{}
	for _, pin := range pins {
		number := retrieval.NormalizeNumber(pin.Number)
		if !validKey(country, pin.Key) || number == "" || keys[pin.Key] || numbers[number] {
			return fmt.Errorf("invalid or duplicate pin %q", pin.Key)
		}
		keys[pin.Key], numbers[number] = true, true
		if _, err := time.Parse("2006-01-02", pin.Since); err != nil {
			return fmt.Errorf("pin %s since: %w", pin.Key, err)
		}
		if pin.Rel == "" || !validHash(pin.Expand) || !validHash(pin.Fetch) {
			return fmt.Errorf("pin %s has invalid acquisition references", pin.Key)
		}
	}
	for _, pin := range pins {
		for _, stage := range []string{"expand", "fetch"} {
			if _, err := Load(ctx, q, artifacts, country, pin, stage); err != nil {
				return err
			}
		}
	}
	return nil
}

func Load(ctx context.Context, q Reader, artifacts artifact.Store, country string, pin source.Pin, stage string) (store.GetPinnedArtifactRow, error) {
	hash := pin.Fetch
	if stage == "expand" {
		hash = pin.Expand
	} else if stage != "fetch" {
		return store.GetPinnedArtifactRow{}, fmt.Errorf("invalid pin stage %q", stage)
	}
	row, err := q.GetPinnedArtifact(ctx, store.GetPinnedArtifactParams{Country: country, Rel: pin.Rel, Stage: stage, ItemKey: pin.Key, Sha256: hash})
	if err != nil {
		return row, fmt.Errorf("pin %s %s producer: %w", pin.Key, stage, err)
	}
	if row.Stage != stage || row.Sha256 != hash {
		return row, fmt.Errorf("pin %s %s producer mismatch", pin.Key, stage)
	}
	number, err := InspectArtifact(artifacts, country, pin.Key, row)
	if err != nil {
		return row, fmt.Errorf("pin %s %s artifact: %w", pin.Key, stage, err)
	}
	if stage == "fetch" && retrieval.NormalizeNumber(number) != retrieval.NormalizeNumber(pin.Number) {
		return row, fmt.Errorf("pin %s number mismatch", pin.Key)
	}
	return row, nil
}

func InspectArtifact(artifacts artifact.Store, country, key string, row store.GetPinnedArtifactRow) (string, error) {
	if !validKey(country, key) {
		return "", fmt.Errorf("invalid pinned document key %q", key)
	}
	var required []string
	switch row.Stage {
	case "expand":
		required = []string{"detail.json", "diagram.json", "observation.json"}
	case "fetch":
		required = []string{"detail.json", "diagram.json", "tree.json", "files.json", "report.json"}
	default:
		return "", fmt.Errorf("invalid pin stage %q", row.Stage)
	}
	dir := artifacts.Abs(row.Path)
	actual, err := artifact.Verify(dir)
	if err != nil {
		return "", fmt.Errorf("verify artifact: %w", err)
	}
	if actual != row.Sha256 {
		return "", fmt.Errorf("artifact hash mismatch")
	}
	if _, err = artifact.VerifyFiles(dir, required...); err != nil {
		return "", fmt.Errorf("verify required files: %w", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "detail.json"))
	if err != nil {
		return "", fmt.Errorf("read pinned detail: %w", err)
	}
	var detail struct{ Data struct{ ID, DocNum string } }
	if err = json.Unmarshal(data, &detail); err != nil {
		return "", fmt.Errorf("decode pinned detail: %w", err)
	}
	if country+":vbpl:"+detail.Data.ID != key {
		return "", fmt.Errorf("detail key mismatch")
	}
	if row.Stage == "fetch" && retrieval.NormalizeNumber(detail.Data.DocNum) == "" {
		return "", fmt.Errorf("empty normalized document number")
	}
	return detail.Data.DocNum, nil
}

func validKey(country, key string) bool {
	prefix := country + ":vbpl:"
	return country != "" && strings.HasPrefix(key, prefix) && validID(strings.TrimPrefix(key, prefix))
}

func validID(id string) bool {
	return len(id) > 0 && strings.ContainsRune("0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ", rune(id[0])) && len(id) <= 64 && !strings.HasPrefix(strings.ToLower(id), "vbpqta_") && strings.Trim(id, "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_-") == ""
}

func validHash(value string) bool {
	_, err := hex.DecodeString(value)
	return len(value) == 64 && strings.ToLower(value) == value && err == nil
}
