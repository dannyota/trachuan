package vbpl

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestDecodeTree_MatchesFixture67878(t *testing.T) {
	nodes, err := DecodeTree(context.Background(), bytes.NewReader(fixture(t, "tree-67878.json")), 32, 50000)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	var walk func([]Node)
	walk = func(nodes []Node) {
		for _, node := range nodes {
			counts[node.Level]++
			walk(node.Children)
		}
	}
	walk(nodes)
	if counts["Article"] != 11 || counts["Clause"] != 34 || counts["Point"] != 17 {
		t.Fatal(counts)
	}
	if len(nodes) == 0 {
		t.Fatal("missing nodes")
	}
	if nodes[0].Title != "Điều 1. Phạm vi điều chỉnh và đối tượng áp dụng" || !strings.HasPrefix(nodes[0].Content, "Điều 1. Phạm vi điều chỉnh và đối tượng áp dụng<br>1. Phạm vi điều chỉnh:") {
		t.Fatalf("first node: %+v", nodes[0])
	}
}

func TestDecodeTree_StopsAtDepthLimit(t *testing.T) {
	body := `{"success":true,"data":[` + strings.Repeat(`{"children":[`, 10000) + strings.Repeat(`]}`, 10000) + `]}`
	start := time.Now()
	_, err := DecodeTree(context.Background(), strings.NewReader(body), 32, 50000)
	if !errors.Is(err, ErrTreeTooLarge) || time.Since(start) >= time.Second {
		t.Fatalf("error = %v, elapsed = %v", err, time.Since(start))
	}
}

func TestDecodeTree_StopsAtNodeLimit(t *testing.T) {
	body := `{"success":true,"data":[` + strings.Repeat(`{},`, 50000) + `{}` + `]}`
	_, err := DecodeTree(context.Background(), strings.NewReader(body), 32, 50000)
	if !errors.Is(err, ErrTreeTooLarge) {
		t.Fatalf("error = %v", err)
	}
}

type cancelOnCheck struct {
	context.Context
	checks int
}

func (c *cancelOnCheck) Err() error {
	c.checks++
	if c.checks > 1 {
		return context.Canceled
	}
	return nil
}

func TestDecodeTree_HonorsContext(t *testing.T) {
	ctx := &cancelOnCheck{Context: context.Background()}
	body := `{"success":true,"data":[` + strings.Repeat(`{},`, 1000) + `{}` + `]}`
	_, err := DecodeTree(ctx, strings.NewReader(body), 32, 50001)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}
