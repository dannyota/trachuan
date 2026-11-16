package taxonomy

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestDrawLabels_DeterministicAndCapped(t *testing.T) {
	var pool []LabelArticle
	for i := 0; i < 200; i++ {
		for j := 0; j < 3; j++ {
			pool = append(pool, LabelArticle{Key: fmt.Sprintf("doc-%03d", i), Anchor: fmt.Sprint(j), Codes: []string{"2"}})
		}
	}
	a, err := DrawLabels(pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	for i, j := 0, len(pool)-1; i < j; i, j = i+1, j-1 {
		pool[i], pool[j] = pool[j], pool[i]
	}
	b, err := DrawLabels(pool, 1)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, b) || len(a) != 300 {
		t.Fatal("draw changed with input order or has wrong size")
	}
	counts := map[string]int{}
	for i, article := range a {
		counts[article.Key]++
		if counts[article.Key] > 2 {
			t.Fatalf("too many from %s", article.Key)
		}
		if i > 0 && (a[i-1].Key > article.Key || (a[i-1].Key == article.Key && a[i-1].Anchor >= article.Anchor)) {
			t.Fatal("output not sorted")
		}
	}
	c, err := DrawLabels(pool, 2)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(a, c) {
		t.Fatal("different seeds gave same draw")
	}
}

func TestDrawLabels_ShortPool(t *testing.T) {
	pool := make([]LabelArticle, 300)
	for i := range pool {
		pool[i] = LabelArticle{Key: "same", Anchor: fmt.Sprint(i)}
	}
	if _, err := DrawLabels(pool, 1); err == nil {
		t.Fatal("expected short eligible pool error")
	}
}

func TestLabels_JSON(t *testing.T) {
	labels := Labels{Country: "vn", Release: "20990101_1", Seed: 1, DrawRule: DrawRuleVersion, Articles: []LabelArticle{{Key: "a", Anchor: "dieu-1", OwnTextSHA256: OwnTextSHA256("abc"), Codes: []string{"1.2"}}}}
	data, err := json.Marshal(labels)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"country":"vn","release":"20990101_1","seed":1,"draw_rule":"seeded-articles/2","articles":[{"document_key":"a","anchor":"dieu-1","own_text_sha256":"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad","codes":["1.2"]}]}`
	if string(data) != want || strings.Contains(string(data), "subset") {
		t.Fatalf("got %s", data)
	}
}
