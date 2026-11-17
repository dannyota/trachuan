package build

import (
	"strings"
	"testing"

	"danny.vn/trachuan/internal/frameworks/registry"
	"github.com/stretchr/testify/require"
)

func caseRegistry() *registry.Registry {
	return &registry.Registry{Frameworks: []registry.Framework{
		{Code: "invented", Version: "r5", Access: "free", Parser: "invented", CitationScheme: "oscal-catalog", Aliases: []string{"Invented Alias"}},
		{Code: "invented", Version: "r5.2", Access: "free", Parser: "invented", CitationScheme: "oscal-catalog"},
		{Code: "invented", Version: "r4", Access: "free", CitationScheme: "oscal-catalog"},
		{Code: "category", Version: "2.0", Access: "licensed", Parser: "invented", CitationScheme: "csf"},
	}}
}

func TestGoldenCases_MapTargetsAndAdjudication(t *testing.T) {
	controls := []Control{
		{Code: "invented", Version: "r5", Citation: "AC-2(3)", CitationNorm: "AC-2(3)"},
		{Code: "invented", Version: "r5.2", Citation: "AC-2(3)", CitationNorm: "AC-2(3)"},
		{Code: "invented", Version: "r5", Citation: "AC", CitationNorm: "AC", Kind: "family"},
		{Code: "category", Version: "2.0", Citation: "GV.OC", CitationNorm: "GV.OC", Kind: "category"},
		{Code: "category", Version: "2.0", Citation: "A #B", CitationNorm: "A#B", Kind: "category"},
	}
	input := "id,question,framework_code,version_label,citation_norm,expect_abstain,notes,adjudication,reason\n" +
		"major,invented query,invented,Revision 5.2.0,AC-02(03),false,,accepted,reviewed\n" +
		"exact,invented query,invented,Rev 5.2,AC-02(03),false,,accepted,reviewed\n" +
		"family,invented query,invented,r5,ac,false,,accepted,reviewed\n" +
		"category,invented query,category,v2.0,GV.OC,false,,unverified,no attestation\n" +
		"outside,invented query,invented,r4,AC-1,false,,accepted,reviewed\n" +
		"unknown-version,invented query,invented,r99,AC-1,false,,out_of_release,not held\n" +
		"negative,invented unrelated query,absent,,,true,,accepted,reviewed negative\n" +
		"rejected,invented query,invented,r5,AC,false,,rejected,wrong target\n" +
		"encoded,invented query,category,2.0,A #B,false,,unverified,no attestation\n"
	cases, err := MapGoldenCases(strings.NewReader(input), caseRegistry(), controls)
	require.NoError(t, err)
	require.Len(t, cases, 9)
	for id, target := range map[string]string{
		"major": "fw:invented:r5#AC-2(3)", "exact": "fw:invented:r5.2#AC-2(3)",
		"family": "fw:invented:r5#AC", "category": "fw:category:2.0#GV.OC",
	} {
		for _, c := range cases {
			if c.ID == id {
				require.Equal(t, target, c.Target)
			}
		}
	}
	require.Equal(t, "unverified", cases[3].Adjudication)
	require.Equal(t, "no attestation", cases[3].Reason)
	require.Equal(t, "out_of_release", cases[4].Adjudication)
	require.Equal(t, "r4", cases[4].Version)
	require.Equal(t, "out_of_release", cases[5].Adjudication)
	require.True(t, cases[6].Negative)
	require.Empty(t, cases[6].Target)
	require.Equal(t, "rejected", cases[7].Adjudication)
	require.Equal(t, "fw:category:2.0#A%20%23B", cases[8].Target)
}

func TestGoldenCases_FailWithCaseIDOnly(t *testing.T) {
	for name, row := range map[string]string{
		"unresolved":   "case-a,private invented question,invented,r5,AC-99,false,,accepted,private reason",
		"alias":        "case-a,private invented question,Invented Alias,r5,AC,false,,accepted,private reason",
		"adjudication": "case-a,private invented question,invented,r5,AC,false,,unknown,private reason",
		"boolean":      "case-a,private invented question,invented,r5,AC,unknown,,accepted,private reason",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := MapGoldenCases(strings.NewReader("id,question,framework_code,version_label,citation_norm,expect_abstain,notes,adjudication,reason\n"+row+"\n"), caseRegistry(), nil)
			require.ErrorContains(t, err, "case-a")
			require.NotContains(t, err.Error(), "private")
		})
	}
}
