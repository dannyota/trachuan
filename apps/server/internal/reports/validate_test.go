package reports

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"danny.vn/trachuan/apps/server/internal/release"
	"danny.vn/trachuan/internal/retrieval"
)

const testKey = "vn:vbpl:03206030-6f7b-11f1-95c5-bd4e620e4897"

func testNow() time.Time { return time.Date(2026, 11, 11, 15, 0, 0, 0, time.UTC) }

type fakeCorpus struct {
	err    error
	number string
}

func (c fakeCorpus) Document(_ context.Context, key string) (Document, bool, error) {
	return Document{Number: "50/2024/TT-NHNN"}, key == testKey, c.err
}
func (c fakeCorpus) AnchorExists(_ context.Context, key, anchor string) (bool, error) {
	return key == testKey && anchor == "dieu-1/khoan-1", c.err
}
func (c fakeCorpus) HasNumber(_ context.Context, number string) (bool, error) {
	return number == "50/2024/TT-NHNN" || number == "196-HDBT" || number == c.number, c.err
}
func (c fakeCorpus) NumberSegments(context.Context) (map[string]struct{}, error) {
	return map[string]struct{}{"TT": {}, "NHNN": {}, "QD": {}, "CP": {}, "50": {}, "2024": {}}, c.err
}

func suggestion(values map[string]string) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(values))
	for k, v := range values {
		out[k], _ = json.Marshal(v)
	}
	return out
}
func validInput(kind string) Input {
	in := Input{Country: "vn", Kind: kind, Handle: testKey}
	if kind == "missing_document" {
		in.Handle = ""
		in.Suggestion = suggestion(map[string]string{"document_number": "1/2025/TT-NHNN"})
	}
	return in
}

func requireCode(t *testing.T, err error, want Error) {
	t.Helper()
	var got Error
	if !errors.As(err, &got) || got != want || err.Error() != want.Error() {
		t.Fatalf("error = %v, want fixed %q", err, want.Error())
	}
}

func TestValidate_KindSuggestionMatrix(t *testing.T) {
	values := map[string]string{"status_class": "partial", "eff_from": "2026-01-01", "eff_to": "2030-01-01", "document_number": "50/2024/TT-NHNN", "issue_date": "2020-01-01"}
	for _, kind := range []string{"wrong_text", "wrong_validity", "broken_citation", "missing_document"} {
		for member, value := range values {
			t.Run(kind+"/"+member, func(t *testing.T) {
				in := validInput(kind)
				if in.Suggestion == nil {
					in.Suggestion = make(map[string]json.RawMessage)
				}
				in.Suggestion[member], _ = json.Marshal(value)
				allowed := kind == "wrong_validity" && (member == "status_class" || member == "eff_from" || member == "eff_to") || kind == "broken_citation" && member == "document_number" || kind == "missing_document" && (member == "document_number" || member == "issue_date")
				_, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
				if allowed {
					if err != nil {
						t.Fatal(err)
					}
					return
				}
				requireCode(t, err, ErrInvalidRequest)
			})
		}
		for _, empty := range []map[string]json.RawMessage{nil, {}} {
			in := validInput(kind)
			in.Suggestion = empty
			_, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
			if kind == "missing_document" {
				requireCode(t, err, ErrInvalidRequest)
			} else if err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, status := range []string{"in_force", "partial", "expired", "not_yet", "suspended", "unknown"} {
		in := validInput("wrong_validity")
		in.Suggestion = suggestion(map[string]string{"status_class": status})
		if _, err := Validate(context.Background(), fakeCorpus{}, in, testNow()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidate_NoFreeTextChannel(t *testing.T) {
	for _, number := range []string{"1/2025/CAN-I-DISMISS-MY-EMPLOYEE", "1/2025/TT-NHNN-CP-QD-TT", "1/1944/TT", "1/2027/TT", "12345/2025/TT", "1/025/TT", "1/2025/TT_ NHNN", "1/2025/TT--NHNN", "1/2025/TT/", "1/2025/TT?private", "1/2025/TT\x00", "", strings.Repeat("X", 129)} {
		t.Run(number, func(t *testing.T) {
			in := validInput("missing_document")
			in.Suggestion = suggestion(map[string]string{"document_number": number})
			_, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
			requireCode(t, err, ErrInvalidRequest)
		})
	}
	for name, raw := range map[string]string{"unknown": `{"note":"PRIVATE"}`, "url": `{"source_url":"PRIVATE"}`, "null": `{"status_class":null}`, "array": `{"status_class":[]}`, "object": `{"status_class":{}}`, "numeric": `{"status_class":1}`, "boolean": `{"status_class":true}`, "sentence": `{"status_class":"PRIVATE"}`, "bad json": `{"status_class":"PRIVATE"}`} {
		t.Run(name, func(t *testing.T) {
			in := validInput("wrong_validity")
			if err := json.Unmarshal([]byte(raw), &in.Suggestion); err != nil {
				t.Fatal(err)
			}
			if name == "bad json" {
				in.Suggestion["status_class"] = json.RawMessage(`"partial" true`)
			}
			_, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
			requireCode(t, err, ErrInvalidRequest)
		})
	}
	for _, body := range []string{
		`{"country":"vn","kind":"wrong_text","handle":"` + testKey + `","source_url":"PRIVATE"}`,
		`{"Country":"vn","kind":"wrong_text"}`, `{"country":"vn","country":"PRIVATE"}`,
		`{"suggestion":{"status_class":"partial","status_class":"PRIVATE"}}`, `{"suggestion":[]}`,
		`{"suggestion":null}`, `{"kind":null}`, `{"handle":3}`, `[]`, `null`,
	} {
		var in Input
		requireCode(t, json.Unmarshal([]byte(body), &in), ErrInvalidRequest)
	}
	for _, change := range []func(*Input){
		func(in *Input) { in.Country = "PRIVATE" }, func(in *Input) { in.Kind = "PRIVATE" },
		func(in *Input) { in.Handle = "vn:PRIVATE#PRIVATE" }, func(in *Input) { in.Handle = strings.Repeat("x", 513) },
		func(in *Input) { in.Handle = testKey + "\x00" }, func(in *Input) { in.Handle = "vn:\xff" },
	} {
		in := validInput("wrong_text")
		change(&in)
		_, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
		requireCode(t, err, ErrInvalidRequest)
	}
}

func TestValidate_Dates(t *testing.T) {
	for _, tc := range []struct {
		member, value string
		valid         bool
	}{
		{"issue_date", "1945-09-02", true}, {"issue_date", "2026-11-11", true}, {"issue_date", "2026-11-12", false},
		{"issue_date", "1945-09-01", false}, {"eff_from", "2027-01-01", true}, {"eff_from", "2036-11-11", true},
		{"eff_to", "2036-11-12", false}, {"eff_to", "1945-09-02", true}, {"eff_from", "1945-09-01", false},
		{"eff_to", "2026-02-29", false}, {"eff_to", "2026-1-01", false}, {"eff_to", "2026-01-01T00:00:00Z", false}, {"eff_to", "", false},
	} {
		t.Run(tc.member+tc.value, func(t *testing.T) {
			kind := "wrong_validity"
			if tc.member == "issue_date" {
				kind = "missing_document"
			}
			in := validInput(kind)
			if in.Suggestion == nil {
				in.Suggestion = make(map[string]json.RawMessage)
			}
			in.Suggestion[tc.member], _ = json.Marshal(tc.value)
			_, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
			if tc.valid {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				requireCode(t, err, ErrInvalidRequest)
			}
		})
	}
	for _, end := range []string{"2026-01-01", "2026-01-02", "2025-12-31"} {
		in := validInput("wrong_validity")
		in.Suggestion = suggestion(map[string]string{"eff_from": "2026-01-01", "eff_to": end})
		_, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
		if end == "2025-12-31" {
			requireCode(t, err, ErrInvalidRequest)
		} else if err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidate_LegacyNumberCitation(t *testing.T) {
	for _, number := range []string{"196-HĐBT", " 196 - hđbt "} {
		in := validInput("broken_citation")
		in.Suggestion = suggestion(map[string]string{"document_number": number})
		report, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
		if err != nil || report.Target != "196-HDBT" || report.Suggestion.DocumentNumber != "196-HDBT" {
			t.Fatalf("report = %+v, err = %v", report, err)
		}
	}
	in := validInput("broken_citation")
	in.Suggestion = suggestion(map[string]string{"document_number": "1/2025/TT"})
	_, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
	requireCode(t, err, ErrInvalidRequest)
	in = validInput("missing_document")
	in.Suggestion = suggestion(map[string]string{"document_number": "196-HĐBT"})
	_, err = Validate(context.Background(), fakeCorpus{}, in, testNow())
	requireCode(t, err, ErrInvalidRequest)
}

func TestValidate_HandleResolves(t *testing.T) {
	for _, handle := range []string{testKey, testKey + "#dieu-1/khoan-1", "vn:unknown", testKey + "#PRIVATE", testKey + "#", testKey + "#dieu-1#PRIVATE", ""} {
		in := validInput("wrong_text")
		in.Handle = handle
		report, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
		if handle == testKey || handle == testKey+"#dieu-1/khoan-1" {
			if err != nil || report.DocumentKey != testKey || report.DocumentNumber != "50/2024/TT-NHNN" {
				t.Fatalf("report = %+v, err = %v", report, err)
			}
		} else {
			requireCode(t, err, ErrInvalidRequest)
		}
	}
	in := validInput("missing_document")
	in.Handle = testKey + "#dieu-1/khoan-1"
	if _, err := Validate(context.Background(), fakeCorpus{}, in, testNow()); err != nil {
		t.Fatal(err)
	}
	in.Handle = testKey
	_, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
	requireCode(t, err, ErrInvalidRequest)
	for _, cause := range []error{release.ErrExpired, ErrReleaseExpired, retrieval.ErrReleaseGone} {
		runner := &fakeRunner{}
		resolver := &fakeResolver{err: cause}
		svc := Service{Store: runner, Resolver: resolver, Clock: testNow}
		_, err := svc.Submit(context.Background(), 1, validInput("wrong_text"))
		requireCode(t, err, ErrReleaseExpired)
		if runner.begins != 0 {
			t.Fatal("retired release began admission")
		}
	}
}

func TestInput_DecodeCanonical(t *testing.T) {
	for _, kind := range []string{"wrong_text", "wrong_validity", "broken_citation", "missing_document"} {
		original := validInput(kind)
		raw, err := json.Marshal(original)
		if err != nil {
			t.Fatal(err)
		}
		var in Input
		if err := json.Unmarshal(raw, &in); err != nil {
			t.Fatal(err)
		}
		if _, err := Validate(context.Background(), fakeCorpus{}, in, testNow()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestValidate_MissingNumberSegments(t *testing.T) {
	for _, number := range []string{"0/1945/TT", "1234/2026/TT-NHNN-QD-CP", "1/2025/TT/NHNN", " 1 / 2025 / tt - nhnn ", "1/2025/50"} {
		in := validInput("missing_document")
		in.Suggestion = suggestion(map[string]string{"document_number": number})
		report, err := Validate(context.Background(), fakeCorpus{}, in, testNow())
		if err != nil || report.Target != retrieval.NormalizeNumber(number) {
			t.Fatalf("report = %+v, err = %v", report, err)
		}
	}
	// Membership, rather than the missing-document grammar, governs citations.
	in := validInput("broken_citation")
	in.Suggestion = suggestion(map[string]string{"document_number": "12345/QD"})
	if _, err := Validate(context.Background(), fakeCorpus{number: "12345/QD"}, in, testNow()); err != nil {
		t.Fatal(err)
	}
}
