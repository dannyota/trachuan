package build

import (
	"strings"
	"testing"

	"golang.org/x/text/unicode/norm"
)

func TestHeadingScan_UnapprovedCandidates(t *testing.T) {
	for _, tc := range []struct {
		heading string
		flagged bool
	}{{"Phạm vi", false}, {"PHẢI làm", true}, {"được làm", true}, {"không được làm", true}, {"có trách nhiệm", true}, {"phải_làm", true}, {"phải2", false}, {"bấtphải", false}, {norm.NFD.String("phải làm"), true}, {"Tên.", false}, {"Tên!", false}, {"Tên?", false}, {"Tên…", true}, {"Tên", false}, {strings.Repeat("ố", 200), false}, {strings.Repeat("ố", 201), true}} {
		d := summaryFixture()
		d.Payload.Outline[0].Heading = tc.heading
		r, _, err := makeUnitSummary(d, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		b, err := encodeHeadingScan("vn", "20990101_1", []summaryDocument{r}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), `"anchor":"chuong-i"`) != tc.flagged {
			t.Fatalf("%q: %s", tc.heading, b)
		}
		again, _ := encodeHeadingScan("vn", "20990101_1", []summaryDocument{r}, nil)
		if string(b) != string(again) {
			t.Fatal("nondeterministic scan")
		}
	}
}
