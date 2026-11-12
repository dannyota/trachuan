package evaluation

import (
	"testing"

	"danny.vn/trachuan/internal/retrieval"
)

func TestMatchProduct_HitInsideExpectedArticle(t *testing.T) {
	article := retrieval.Identity{Country: "vn", DocumentKey: "vn:vbpl:171687", Anchor: "dieu-10"}
	child := retrieval.Identity{Country: article.Country, DocumentKey: article.DocumentKey, Anchor: "dieu-10/khoan-1"}
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Kind: "provision", Country: article.Country, DocumentKey: article.DocumentKey, Anchor: &article.Anchor}}}
	hit := retrieval.Hit{Identity: child, Path: []retrieval.PathNode{{Identity: article}}, Fragments: []retrieval.Fragment{{Identity: article, Complete: false}, {Identity: article, Complete: false}}}
	if got := MatchProduct(entry, []retrieval.Hit{{}, hit, hit}); got != 2 {
		t.Fatalf("rank = %d, want 2", got)
	}
}

func TestMatchProduct_ParentHitDoesNotSatisfyChild(t *testing.T) {
	parent := retrieval.Identity{Country: "vn", DocumentKey: "doc", Anchor: "dieu-10"}
	child := retrieval.Identity{Country: "vn", DocumentKey: "doc", Anchor: "dieu-10/khoan-1"}
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Kind: "provision", Country: "vn", DocumentKey: "doc", Anchor: &child.Anchor}}}
	hit := retrieval.Hit{Identity: parent, Fragments: []retrieval.Fragment{{Identity: child}}}
	if got := MatchProduct(entry, []retrieval.Hit{hit}); got != 0 {
		t.Fatalf("rank = %d, want 0", got)
	}
}

func TestMatchProduct_OtherDocumentSameAnchorNoMatch(t *testing.T) {
	anchor := "dieu-10"
	entry := MapEntry{State: "resolved", Alternatives: []Alternative{{Kind: "provision", Country: "vn", DocumentKey: "doc", Anchor: &anchor}}}
	for _, hit := range []retrieval.Hit{
		{Identity: retrieval.Identity{Country: "vn", DocumentKey: "other", Anchor: anchor}},
		{Identity: retrieval.Identity{Country: "vn", DocumentKey: "other", Anchor: "child"}, Path: []retrieval.PathNode{{Identity: retrieval.Identity{Country: "vn", DocumentKey: "other", Anchor: anchor}}}},
		{Identity: retrieval.Identity{Country: "vn", DocumentKey: "doc", Anchor: "child"}, Path: []retrieval.PathNode{{Identity: retrieval.Identity{Country: "vn", DocumentKey: "other", Anchor: anchor}}}},
	} {
		if got := MatchProduct(entry, []retrieval.Hit{hit}); got != 0 {
			t.Fatalf("rank = %d for %+v, want 0", got, hit)
		}
	}
}
