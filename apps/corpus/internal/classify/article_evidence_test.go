package classify

import (
	"testing"

	"danny.vn/trachuan/apps/corpus/internal/taxonomy"
)

func TestSelectArticles_OrdersAndIncludesDescendantsWithoutLabels(t *testing.T) {
	provisions := []ProvisionEvidence{{DocumentKey: "vn:vbpl:1", Kind: "chuong", Path: "n1", Anchor: "chuong-1"}, {DocumentKey: "vn:vbpl:1", Kind: "dieu", Path: "n1.n2", Anchor: "dieu-2", Text: "article own", Heading: "Heading"}, {DocumentKey: "vn:vbpl:1", Kind: "khoan", Path: "n1.n2.n3", Text: "clause"}, {DocumentKey: "vn:vbpl:1", Kind: "diem", Path: "n1.n2.n3.n4", Text: "point"}, {DocumentKey: "vn:vbpl:1", Kind: "dieu", Path: "n1.n5", Anchor: "dieu-10", Text: "next"}, {DocumentKey: "vn:vbpl:1", Kind: "dieu", Path: "n1.n6", Anchor: "dieu-11", Text: "unselected"}}
	labels := []taxonomy.LabelArticle{{Key: "vn:vbpl:1", Anchor: "dieu-10", Codes: []string{"hidden"}}, {Key: "vn:vbpl:1", Anchor: "dieu-2", Codes: []string{"hidden"}}, {Key: "missing", Anchor: "dieu-1"}}
	articles, err := SelectArticles(provisions, labels)
	if err != nil || len(articles) != 2 || articles[0].Anchor != "dieu-2" || articles[0].OwnText != "article own" || articles[0].Text != "article own\nclause\npoint" || articles[1].Chapter != "n1" {
		t.Fatalf("articles %+v error %v", articles, err)
	}
}
