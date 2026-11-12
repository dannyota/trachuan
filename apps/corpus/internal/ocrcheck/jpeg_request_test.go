package ocrcheck

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

type reusedPageClient struct {
	*FakeClient
	t     *testing.T
	first *anthropic.ImageBlockParam
	paths []string
}

func (c *reusedPageClient) check(r Request) {
	c.t.Helper()
	p := messageParams(r)
	first := p.Messages[0].OfUser.Content[1].OfImage
	if c.first == nil {
		c.first = first
	}
	if first != c.first {
		c.t.Error("page blocks rebuilt")
	}
	if len(r.Images) != 0 {
		c.t.Error("raw JPEG pages retained beside encoded blocks")
	}
	// Sizing must consume the same blocks without changing or copying payloads.
	size, err := jpegRequestSize(r)
	encoded, marshalErr := json.Marshal(p)
	if err != nil || marshalErr != nil || size != len(encoded)+len(`,"stream":true`) {
		c.t.Errorf("size=%d want=%d errors=%v/%v", size, len(encoded)+len(`,"stream":true`), err, marshalErr)
	}
	if messageParams(r).Messages[0].OfUser.Content[1].OfImage != first {
		c.t.Error("sizing changed the cached page")
	}
}
func (c *reusedPageClient) CountTokens(ctx context.Context, r Request) (int64, error) {
	c.check(r)
	c.check(r)
	for _, path := range c.paths {
		if err := os.Remove(path); err != nil {
			c.t.Error(err)
		}
	}
	return c.FakeClient.CountTokens(ctx, r)
}
func (c *reusedPageClient) Direct(ctx context.Context, r Request) (Response, error) {
	c.check(r)
	return c.FakeClient.Direct(ctx, r)
}

func TestJPEGRequest_ReadsAndEncodesOnce(t *testing.T) {
	for _, version := range []int{3, 4} {
		t.Run(fmt.Sprint(version), func(t *testing.T) {
			r, _, old, docs, f := jpegFixture(t)
			reads := map[string]int{}
			encodes := 0
			r.readJPEG = func(path string) ([]byte, error) { reads[path]++; return os.ReadFile(path) }
			r.encodeJPEG = func(page []byte) string { encodes++; return base64.StdEncoding.EncodeToString(page) }
			c := &reusedPageClient{FakeClient: old, t: t, paths: f.paths}
			r.Client = c
			o := jpegOptions()
			o.CheckVersion = version
			if err := run(t, r, docs, o); err != nil {
				t.Fatal(err)
			}
			for _, path := range f.paths {
				if reads[path] != 1 {
					t.Errorf("page reads=%d want=1", reads[path])
				}
			}
			if encodes != 10 || c.CountCalls != 1 || c.DirectCalls != 1 || !f.cleaned {
				t.Fatalf("encodes=%d count=%d direct=%d cleanup=%v", encodes, c.CountCalls, c.DirectCalls, f.cleaned)
			}
		})
	}
}

func TestJPEGRequest_SizeMatchesJSON(t *testing.T) {
	for _, text := range []string{"public OCR", "<>&\"\\\n\t\b\f\r\x00\x01", "Điều 1: Quy định\u2028\u2029", string([]byte{0xff, 0xfe, 'a', 0xc0})} {
		r, _, old, docs, f := jpegFixture(t)
		docs[0].Text = []byte(text)
		r.Client = &reusedPageClient{FakeClient: old, t: t, paths: f.paths}
		if err := run(t, r, docs, jpegOptions()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJSONStringSize_MatchesSDKEncoding(t *testing.T) {
	all := make([]byte, 256)
	for i := range all {
		all[i] = byte(i)
	}
	texts := []string{string([]byte{0xff}), string(all), "", "\ufffd\u2028\u2029", "Điều khoản", string([]byte{0xe2, 0x80, 'x', 0xf0, 0x80, 0x80, 0x80})}
	empty, err := json.Marshal(anthropic.NewTextBlock(""))
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range texts {
		encoded, err := json.Marshal(anthropic.NewTextBlock(text))
		if err != nil {
			t.Fatal(err)
		}
		if size := jsonStringSize(text); size != len(encoded)-len(empty)+2 {
			t.Fatalf("size=%d encoded=%d", size, len(encoded)-len(empty)+2)
		}
	}
}
