package source

import "testing"

func TestKeywordSearchIdentity_Definition(t *testing.T) {
	a := KeywordSearchIdentity("chinhphu", "trí tuệ nhân tạo", 500)
	if a == "" || a != KeywordSearchIdentity("chinhphu", "trí tuệ nhân tạo", 500) || a == KeywordSearchIdentity("chinhphu", "trí tuệ nhân tạo", 100) || a == KeywordSearchIdentity("chinhphu", "other", 500) || a == KeywordSearchIdentity("other", "trí tuệ nhân tạo", 500) {
		t.Fatal("identity must bind source, keyword and cap")
	}
}
