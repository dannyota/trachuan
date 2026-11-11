//go:build integration

package ocrcheck

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"
)

func TestCheck_CountsPDFPageTree(t *testing.T) {
	var pdf bytes.Buffer
	pdf.WriteString("%PDF-1.7\n")
	offsets := []int{0}
	for i, object := range []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << >> >>",
	} {
		offsets = append(offsets, pdf.Len())
		_, _ = fmt.Fprintf(&pdf, "%d 0 obj\n%s\nendobj\n", i+1, object)
	}
	xref := pdf.Len()
	pdf.WriteString("xref\n0 4\n0000000000 65535 f \n")
	for _, offset := range offsets[1:] {
		_, _ = fmt.Fprintf(&pdf, "%010d 00000 n \n", offset)
	}
	_, _ = fmt.Fprintf(&pdf, "trailer\n<< /Size 4 /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", xref)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pages, err := recoverCount(ctx, countPages, pdf.Bytes())
	if err != nil || pages != 1 {
		t.Fatalf("pages=%d error=%v", pages, err)
	}
}
