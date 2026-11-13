package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/source"
	"danny.vn/trachuan/apps/corpus/internal/stage/extract"
)

func TestHeadingCorrections_CommandUsage(t *testing.T) {
	for _, args := range [][]string{{"heading-corrections"}, {"heading-corrections", "vn", "20990101", "--unknown"}} {
		var out, errout bytes.Buffer
		if code := Run(context.Background(), args, &out, &errout); code != 2 || out.Len() != 0 || errout.Len() == 0 {
			t.Fatalf("code=%d stdout=%s stderr=%s", code, &out, &errout)
		}
	}
}

func TestHeadingCorrections_MissingQuoteFailsPreflight(t *testing.T) {
	for _, found := range []bool{true, false} {
		var out, errout bytes.Buffer
		checks := []extract.HeadingCorrectionCheck{{Key: "vn:vbpl:2145", EvidenceKey: "vn:vbpl:10761", Anchor: "dieu-20", QuoteFound: found}}
		code := writeHeadingCorrections(&out, &errout, checks)
		var got []extract.HeadingCorrectionCheck
		if err := json.Unmarshal(out.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0] != checks[0] || (code == 0) != found {
			t.Fatalf("code=%d output=%s stderr=%s", code, &out, &errout)
		}
		if !found && !strings.Contains(errout.String(), "update corrections before the run") {
			t.Fatalf("stderr=%s", &errout)
		}
	}
}

func TestHeadingCorrections_ExternalEvidenceOnSwitchingDocument(t *testing.T) {
	data, err := os.ReadFile("../stage/extract/testdata/heading-5664.json")
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Tree json.RawMessage `json:"tree"`
		HTML string          `json:"html"`
	}
	if err = json.Unmarshal(data, &f); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err = os.Mkdir(filepath.Join(dir, "files"), 0700); err != nil {
		t.Fatal(err)
	}
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(f.HTML)))
	for name, value := range map[string][]byte{
		"tree.json": f.Tree, "detail.json": []byte(`{"data":{"id":"1"}}`),
		"files.json":    fmt.Appendf(nil, `[{"name":"1_content.html","sha256":"%s","size":%d}]`, hash, len(f.HTML)),
		"files/" + hash: []byte(f.HTML),
	} {
		if err = os.WriteFile(filepath.Join(dir, name), value, 0600); err != nil {
			t.Fatal(err)
		}
	}
	entry := source.Correction{Key: "vn:vbpl:1", Evidence: source.CorrectionEvidence{URL: "https://example.org/evidence", Quote: "external evidence"}}
	checks, err := extract.CheckHeadingCorrections(context.Background(), []source.Correction{entry}, map[string]string{entry.Key: dir})
	if err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	code := writeHeadingCorrections(&out, &errout, checks)
	if len(checks) != 0 || code != 0 || strings.TrimSpace(out.String()) != "[]" || errout.Len() != 0 {
		t.Fatalf("checks=%+v code=%d stdout=%s stderr=%s", checks, code, &out, &errout)
	}
}
