package password

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

func TestHash_RoundTripAndBounds(t *testing.T) {
	h := NewHasher()
	encoded, err := h.Hash(context.Background(), "a long synthetic password")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(encoded, "$argon2id$v=19$m=65536,t=3,p=2$") {
		t.Fatal("wrong parameters")
	}
	for _, tc := range []struct {
		input string
		match bool
	}{{"a long synthetic password", true}, {"another synthetic password", false}} {
		match, err := h.Verify(context.Background(), encoded, tc.input)
		if err != nil || match != tc.match {
			t.Fatalf("match=%v error=%v", match, err)
		}
	}
	for _, bad := range []string{"", strings.Replace(encoded, "m=65536", "m=999999999", 1), strings.Replace(encoded, "p=2", "p=0", 1), encoded + "!"} {
		if _, err := h.Verify(context.Background(), bad, "synthetic"); !errors.Is(err, ErrHashInvalid) {
			t.Fatal("accepted malformed hash")
		}
	}
	if match, err := h.Verify(context.Background(), "", "synthetic"); match || !errors.Is(err, ErrHashInvalid) {
		t.Fatal("bad hash")
	}
	if err := h.VerifyDummy(context.Background(), "synthetic"); err != nil {
		t.Fatal(err)
	}
}

func TestPolicy_NormalizationAndRejection(t *testing.T) {
	for _, tc := range []struct {
		raw, email string
		valid      bool
	}{
		{"abcdefghijklmn", "person@example.test", false},
		{strings.Repeat("a", 129), "person@example.test", false},
		{strings.Repeat("a", 1025), "person@example.test", false},
		{"123456789012345", "person@example.test", false},
		{"longlocalpart123@example.test", "longlocalpart123@example.test", false},
		{"longlocalpart123", "longlocalpart123@example.test", false},
		{"Longlocalpart123", "longlocalpart123@example.test", true},
		{"a synthetic passphrase", "person@example.test", true},
		{strings.Repeat("e\u0301", 15), "person@example.test", true},
		{strings.Repeat("a", 15) + "\xff", "person@example.test", false},
	} {
		got, err := Check(tc.raw, tc.email)
		if (err == nil) != tc.valid {
			t.Errorf("valid=%v error=%v", tc.valid, err)
		}
		if err == nil && !norm.NFC.IsNormalString(got) {
			t.Fatal("not NFC")
		}
	}
}

func TestBlocklist_PinAndEntries(t *testing.T) {
	license, err := bundled.ReadFile("blocklist/LICENSE")
	if err != nil || !strings.Contains(string(license), "Permission is hereby granted") {
		t.Fatal("missing embedded MIT license")
	}
	digest := sha256.Sum256([]byte(commonPasswords()))
	if hex.EncodeToString(digest[:]) != "8683ed5b2e4837b733f5a3f4332a1ee0eaebfaf04e03c6b92918fcb664e45cce" {
		t.Fatal("blocklist drift")
	}
	for entry := range strings.SplitSeq(strings.TrimSuffix(commonPasswords(), "\n"), "\n") {
		if !norm.NFC.IsNormalString(entry) || utf8.RuneCountInString(entry) < 15 || utf8.RuneCountInString(entry) > 128 || len(entry) > 1024 {
			t.Fatal("invalid entry")
		}
		if _, err := Check(entry, "person@example.test"); err == nil {
			t.Fatal("entry accepted")
		}
	}
}

func TestAdmission_BoundsTimeoutAndCancellation(t *testing.T) {
	a := NewAdmission()
	ctx := context.Background()
	for range 2 {
		if err := a.Acquire(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for range 16 {
		a.queue <- struct{}{}
	}
	if err := a.Acquire(ctx); !errors.Is(err, ErrBusy) {
		t.Fatal("queue not bounded")
	}
	for range 16 {
		<-a.queue
	}
	a.wait = time.Nanosecond
	if err := a.Acquire(ctx); !errors.Is(err, ErrBusy) {
		t.Fatal("wait not bounded")
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if err := a.Acquire(canceled); !errors.Is(err, ErrBusy) {
		t.Fatal("cancellation not busy")
	}
	a.Release()
	a.Release()
	if err := a.Acquire(ctx); err != nil {
		t.Fatal(err)
	}
	a.Release()
}
