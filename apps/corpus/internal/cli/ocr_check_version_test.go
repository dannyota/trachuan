package cli

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheck_VersionOptions(t *testing.T) {
	for _, review := range []bool{false, true} {
		for _, version := range []string{"2", "3", "4", "1", "0", "bad"} {
			t.Run(fmt.Sprintf("%v/%s", review, version), func(t *testing.T) {
				args := []string{"--check-version", version}
				if !review {
					args = append(args, "--max-usd", "1")
				}
				o, err := parseCheckOptions(args, review)
				valid := version == "2" || version == "3" || version == "4"
				if (err == nil) != valid {
					t.Fatalf("options=%+v err=%v", o, err)
				}
				if valid && !strings.HasSuffix(o.CheckerID(), "ocr-check-"+version) {
					t.Fatal(o.CheckerID())
				}
			})
		}
	}
	for _, version := range []string{"3", "4"} {
		if _, err := parseCheckOptions([]string{"--check-version", version, "--max-usd", "1", "--batch"}, false); err == nil {
			t.Fatal("accepted JPEG batch")
		}
	}
}
