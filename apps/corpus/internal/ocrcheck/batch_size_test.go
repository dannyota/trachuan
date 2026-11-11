package ocrcheck

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
)

type encodedBatchClient struct {
	*FakeClient
	t     *testing.T
	limit int
	sizes []int
}

func (c *encodedBatchClient) CreateBatch(ctx context.Context, requests []Request) (Batch, error) {
	params := anthropic.MessageBatchNewParams{}
	for _, request := range requests {
		params.Requests = append(params.Requests, batchRequest(request))
	}
	encoded, err := json.Marshal(params)
	if err != nil {
		c.t.Fatal(err)
	}
	c.sizes = append(c.sizes, len(encoded))
	if len(encoded) > c.limit {
		c.t.Fatalf("batch payload exceeded limit: bytes=%d limit=%d", len(encoded), c.limit)
	}
	return c.FakeClient.CreateBatch(ctx, requests)
}
func TestCheck_BatchSplitsByEncodedSize(t *testing.T) {
	r, _, fake, docs := fixture(t)
	docs[0].Text = []byte(strings.Repeat("quote\" <tag> & newline\n", 4000))
	second := docs[0]
	second.Entry.ID = 2
	docs = append(docs, second)
	pdf, err := os.ReadFile(docs[0].PDFPath)
	if err != nil {
		t.Fatal(err)
	}
	request := Request{Model: options().Model, Effort: options().Effort, CustomID: strings.Repeat("a", 32) + "-9223372036854775807", PDF: pdf, Text: docs[0].Text}
	one, err := json.Marshal(anthropic.MessageBatchNewParams{Requests: []anthropic.MessageBatchNewParamsRequest{batchRequest(request)}})
	if err != nil {
		t.Fatal(err)
	}
	r.batchBytes = len(one) + 64
	client := &encodedBatchClient{FakeClient: fake, t: t, limit: r.batchBytes}
	r.Client = client
	o := options()
	o.Batch = true
	if err = run(t, r, docs, o); err != nil {
		t.Fatal(err)
	}
	if len(client.sizes) != 2 {
		t.Fatalf("encoded requests did not split: batches=%d", len(client.sizes))
	}
}
