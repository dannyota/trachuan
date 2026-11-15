//go:build integration

package build

import (
	"context"
	"fmt"
	"testing"
)

func TestLoad_NormalizesDocumentLabels(t *testing.T) {
	f := fixture(t)
	cases := []struct{ docType, issuer, wantType, wantIssuer string }{
		{"\n   Quyết định", "\n  Chính phủ", "Quyết định", "Chính phủ"},
		{" Quye\u0302\u0301t\t đi\u0323nh ", " Chi\u0301nh  phu\u0309 ", "Quyết định", "Chính phủ"},
		{" \n\t", "\u2003 ", "", ""},
	}
	for i, tc := range cases {
		id := fmt.Sprint(i + 1)
		f.addArtifact(t, "fetch", "vn:vbpl:"+id, map[string]any{"detail.json": map[string]any{"data": map[string]any{"id": id, "docType": map[string]any{"name": tc.docType}, "agencyName": tc.issuer}}, "files.json": []any{}})
	}
	if _, err := f.load(t); err != nil {
		t.Fatal(err)
	}
	for i, tc := range cases {
		var docType, issuer string
		if err := f.corpus.QueryRow(context.Background(), `SELECT doc_type,issuer FROM document WHERE key=$1`, fmt.Sprintf("vn:vbpl:%d", i+1)).Scan(&docType, &issuer); err != nil {
			t.Fatal(err)
		}
		if docType != tc.wantType || issuer != tc.wantIssuer {
			t.Errorf("labels = %q, %q; want %q, %q", docType, issuer, tc.wantType, tc.wantIssuer)
		}
	}
}
