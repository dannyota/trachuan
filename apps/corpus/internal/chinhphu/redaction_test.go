package chinhphu

import (
	"bytes"
	"context"
	"net/http"
	"testing"
)

func TestRedact_FieldBasedAndHistoryIndependent(t *testing.T) {
	c := New()
	prior, err := parseHTML([]byte(`<input name="__VIEWSTATE" value="shared">`))
	if err != nil {
		t.Fatal(err)
	}
	c.form(prior)
	pages := []string{
		`<input name="__VIEWSTATE" value="shared"><input name="__EVENTVALIDATION" value="shared"><p>shared</p>`,
		`<input name="__VIEWSTATE" value="abc"><input name="__EVENTVALIDATION" value="abcd"><p>shared</p>`,
	}
	want := `<input name="__VIEWSTATE" value="REDACTED-__VIEWSTATE"><input name="__EVENTVALIDATION" value="REDACTED-__EVENTVALIDATION"><p>shared</p>`
	for _, page := range pages {
		if got := string(c.redact([]byte(page))); got != want {
			t.Errorf("redact = %q, want %q", got, want)
		}
	}
	t.Run("prior requests", func(t *testing.T) {
		tokens := []string{"previous-token", "abc", "longer-token-value"}
		i := 0
		client, _ := clientFor(t, func(w http.ResponseWriter, _ *http.Request) {
			body := bytes.ReplaceAll(fixture(t, "detail-218029.html"), []byte("REDACTED-__VIEWSTATE"), []byte(tokens[i]))
			i++
			_, _ = w.Write(body)
		})
		if _, _, err := client.Detail(context.Background(), 218029); err != nil {
			t.Fatal(err)
		}
		_, first, err := client.Detail(context.Background(), 218029)
		if err != nil {
			t.Fatal(err)
		}
		_, second, err := client.Detail(context.Background(), 218029)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(first, second) {
			t.Fatal("token rotation or request history changed redacted bytes")
		}
	})
}

func TestRedact_DoesNotAlterOtherAttributes(t *testing.T) {
	c := New()
	input := `<input title=" value='ordinary' " name="__VIEWSTATE" value="secret">`
	want := `<input title=" value='ordinary' " name="__VIEWSTATE" value="REDACTED-__VIEWSTATE">`
	if got := string(c.redact([]byte(input))); got != want {
		t.Fatalf("redact = %q, want %q", got, want)
	}
}

func TestRedact_AfterBooleanAttribute(t *testing.T) {
	c := New()
	input := `<input disabled value="secret" name="__VIEWSTATE">`
	want := `<input disabled value="REDACTED-__VIEWSTATE" name="__VIEWSTATE">`
	if got := string(c.redact([]byte(input))); got != want {
		t.Fatalf("redact = %q, want %q", got, want)
	}
}

func TestRedact_EscapesFieldName(t *testing.T) {
	c := New()
	input := `<input name="__VIEWSTATE&amp;quot;" value="secret">`
	got := string(c.redact([]byte(input)))
	want := `<input name="__VIEWSTATE&amp;quot;" value="REDACTED-__VIEWSTATE&amp;quot;">`
	if got != want {
		t.Fatalf("redact = %q, want %q", got, want)
	}
}

func TestRedact_PreservesTrailingMalformedText(t *testing.T) {
	c := New()
	input := `<input name="__VIEWSTATE" value="secret">unclosed<`
	want := `<input name="__VIEWSTATE" value="REDACTED-__VIEWSTATE">unclosed<`
	if got := string(c.redact([]byte(input))); got != want {
		t.Fatalf("redact = %q, want %q", got, want)
	}
}
