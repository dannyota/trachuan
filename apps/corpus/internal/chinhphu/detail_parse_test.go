package chinhphu

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"testing"
)

func TestParseDetail_MatchesClientDetail(t *testing.T) {
	c, _ := clientFor(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(fixture(t, "detail-218029.html")) })
	want, raw, err := c.Detail(context.Background(), 218029)
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.ParseDetail(raw, want.URL)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("parsed=%+v want=%+v err=%v", got, want, err)
	}
	if _, err = c.ParseDetail(fixture(t, "detail-missing.html"), want.URL); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing error=%v", err)
	}
}
