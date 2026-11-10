package source

import "testing"

func TestQueryIdentity_CentralListingAndLookup(t *testing.T) {
	if got := CentralListingIdentity("vbpl", "TRUNG_UONG", 100); got != "662df568be7a22599a48a18a5a638d8ba260015cb3babd6e526e0e5f542ba15d" {
		t.Errorf("listing = %s", got)
	}
	if got := NamedLookupIdentity("vbpl", "docNum", "exact", "TRUNG_UONG", "71/2025/QH15"); got != "0bceed38d3b9efcc1c76e17e38dd9d6ec5b9707910c738abecf8fdba72765078" {
		t.Errorf("lookup = %s", got)
	}
}
