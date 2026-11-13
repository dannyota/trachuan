package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

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
