package evidence

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestWriteIdentity_Golden(t *testing.T) {
	src, m := currentSummary()
	var buf bytes.Buffer
	if err := writeIdentity(context.Background(), &buf, src, m, identityTestBindings(testRel)); err != nil {
		t.Fatal(err)
	}
	golden(t, "identity.jsonl", buf.Bytes())
}

func TestWriteIdentity_DeterministicToTheByte(t *testing.T) {
	src, m := currentSummary()
	var first, second bytes.Buffer
	if err := writeIdentity(context.Background(), &first, src, m, identityTestBindings(testRel)); err != nil {
		t.Fatal(err)
	}
	src.relations = append(src.relations[2:], src.relations[:2]...)
	src.documents = append(src.documents[3:], src.documents[:3]...)
	if err := writeIdentity(context.Background(), &second, src, m, identityTestBindings(testRel)); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first.Bytes(), second.Bytes()) {
		t.Fatalf("input order changed the bytes:\n%s\n%s", first.Bytes(), second.Bytes())
	}
}

func TestWriteIdentity_AbsentMembersAndEveryLineBounded(t *testing.T) {
	src, m := currentSummary()
	var buf bytes.Buffer
	if err := writeIdentity(context.Background(), &buf, src, m, identityTestBindings(testRel)); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if !strings.Contains(text, `{"type":"document","key":"vn:vbpl:2099006","absent":true}`) {
		t.Fatalf("no absent line for an active member without a row:\n%s", text)
	}
	if strings.Contains(text, "vn:vbpl:2099002") || strings.Contains(text, "vn:chinhphu:2099003\"") {
		t.Fatalf("removed members listed:\n%s", text)
	}
	if !strings.HasSuffix(text, "\n") {
		t.Fatal("file does not end with a newline")
	}
}

func TestWriteIdentity_RefusesOverlongLine(t *testing.T) {
	src, m := currentSummary()
	long := strings.Repeat("x", lineLimit)
	src.relations = append(src.relations, rel("cites", "vbpl_diagram", "vn:vbpl:2099001", long, nil, "", "x", false, nil, nil))
	var buf bytes.Buffer
	err := writeIdentity(context.Background(), &buf, src, m, identityTestBindings(testRel))
	var refused *inputError
	if !errors.As(err, &refused) || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("err = %v", err)
	}
}

type unorderedSummary struct{ *fakeSummary }

func (u unorderedSummary) SummaryDocuments(ctx context.Context) ([]summaryDocument, error) {
	docs, err := u.fakeSummary.SummaryDocuments(ctx)
	docs[0], docs[1] = docs[1], docs[0]
	return docs, err
}

type orphanSummary struct{ *fakeSummary }

func (o orphanSummary) SummaryProvisions(_ context.Context, fn func(string, []summaryProvision) error) error {
	return fn("vn:vbpl:2099999", []summaryProvision{prov(1, 0, "dieu-1", "x")})
}

func TestWriteIdentity_RefusesSourcesOutOfOrderOrUnbound(t *testing.T) {
	src, m := currentSummary()
	for name, source := range map[string]summarySource{"documents out of order": unorderedSummary{src}, "provision without a document": orphanSummary{src}} {
		t.Run(name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := writeIdentity(context.Background(), &buf, source, m, identityTestBindings(testRel)); err == nil {
				t.Fatal("accepted")
			}
		})
	}
}

func TestWriteIdentity_SourceErrorsPropagate(t *testing.T) {
	src, m := currentSummary()
	src.err = errors.New("connection reset")
	var buf bytes.Buffer
	if err := writeIdentity(context.Background(), &buf, src, m, identityTestBindings(testRel)); err == nil || !strings.Contains(err.Error(), "connection reset") {
		t.Fatalf("err = %v", err)
	}
}
