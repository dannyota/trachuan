package retrieval

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestNext_RoundTrip(t *testing.T) {
	want := cursor{V: 1, Release: "release", Item: ReadItem{Handle: "vn:key", From: "a", To: "b", Relations: true}, Unit: 2, Part: 3}
	token := encodeNext(want)
	if strings.Contains(token, "=") {
		t.Fatal("token contains padding")
	}
	got, err := decodeNext(token)
	if err != nil || got != want {
		t.Fatalf("decodeNext() = %+v, %v; want %+v", got, err, want)
	}
	data, err := json.Marshal(ReadItem{Handle: "vn:key"})
	if err != nil || string(data) != `{"handle":"vn:key"}` {
		t.Fatalf("ReadItem JSON = %s, %v", data, err)
	}
}

func TestDecodeNext_RejectsTampered(t *testing.T) {
	encode := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	tests := map[string]string{
		"base64":        "!",
		"unknown field": encode(`{"V":1,"Item":{"Handle":"vn:key"},"extra":1}`),
		"version":       encode(`{"V":2,"Item":{"Handle":"vn:key"}}`),
		"negative unit": encode(`{"V":1,"Item":{"Handle":"vn:key"},"Unit":-1}`),
		"negative part": encode(`{"V":1,"Item":{"Handle":"vn:key"},"Part":-1}`),
		"nested next":   encode(`{"V":1,"Item":{"Handle":"vn:key","Next":"nested"}}`),
		"both forms":    encode(`{"V":1,"Item":{"Handle":"vn:key","Number":"number"}}`),
		"trailing data": encode(`{"V":1,"Item":{"Handle":"vn:key"}} {}`),
		"invalid UTF-8": encode("{\"V\":1,\"Item\":{\"handle\":\"vn:\xff\"}}"),
	}
	for name, input := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := decodeNext(input); !errors.Is(err, errInvalidNext) {
				t.Fatalf("decodeNext() error = %v, want errInvalidNext", err)
			}
		})
	}
}
