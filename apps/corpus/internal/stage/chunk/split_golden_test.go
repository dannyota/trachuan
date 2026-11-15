package chunk

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"testing"
)

func TestSplit_Golden(t *testing.T) {
	article := "article"
	for _, tc := range []struct {
		name     string
		nodes    []Node
		file     string
		coverage string
	}{
		{
			name:     "fits whole verbatim",
			nodes:    []Node{{NodeID: article, Kind: "other", Path: "preamble", Text: "  chữ  Việt\n\tgiữ. \n"}},
			file:     "07b56dadc9c9818450854198cab0cab4af596fe34ede3838de60c4be3f44ebe0",
			coverage: "551d8b6776fdc84beca897b94c1e007e3fb76f708e84ae8f2e04e0a44e3d56a7",
		},
		{
			name:     "packed lines and blanks",
			nodes:    []Node{{NodeID: article, Kind: "dieu", Path: "dieu-1", Text: "  alpha \t\n beta\n\n\u2003gamma\n delta  \n epsilon \n zeta\t"}},
			file:     "a7d587d168dc6d89fd87b04e0eb7dc7fe8450021e56d1d4940beafecbf3158d6",
			coverage: "e5c610ab7ef0f9ed337ea491849d7a69930b9b1fcaac180501aaa952d227b8d5",
		},
		{
			name:     "packed words and oversized word",
			nodes:    []Node{{NodeID: article, Kind: "dieu", Path: "dieu-1", Text: "một\t hai  ba\u2003bốn năm sáu bảy tám chín abcdefghijklmnopqrstuvwxyz cuối"}},
			file:     "b6855a8832de816abe3f26505aafc95b17748e6df91fa721209080dbd05a4295",
			coverage: "72c8bcd69685dba452f186f81276c798924a33de536ebad2cac9eb30662247c5",
		},
		{
			name: "attached lead and containment aliases",
			nodes: []Node{
				{NodeID: article, Kind: "dieu", Path: "dieu-1", Text: "Chung:\nTrách nhiệm một.\nTrách nhiệm hai."},
				{NodeID: "one", ParentNodeID: &article, Kind: "khoan", Path: "dieu-1/khoan-1", Text: "Trách nhiệm một."},
				{NodeID: "two", ParentNodeID: &article, Kind: "khoan", Path: "dieu-1/khoan-2", Ordinal: 1, Text: "Trách nhiệm hai."},
			},
			file:     "870fdabc8bb11acb4ae784be45ddea43dd3e76e029370dc60dad42c200dd55e6",
			coverage: "53d428c075437b46f35d8007a15c9c51cedb2492e805912dfa4a719aa3ee4bf6",
		},
		{
			name: "lead leaves no room for word",
			nodes: []Node{
				{NodeID: article, Kind: "dieu", Path: "dieu-1", Text: "Quy định"},
				{NodeID: "one", ParentNodeID: &article, Kind: "khoan", Path: "dieu-1/khoan-1", Text: "aa bb abcdefghijklmnopqr cc dd"},
			},
			file:     "ae5ff7c66d88c60076be7e973befd1539348718a3ca99bfd9d624d92e4937de9",
			coverage: "aa2110c801fb9ad58b206b928914c45f5f0f82bc55607c6bc6dc000147fbf928",
		},
		{
			name:     "whitespace only",
			nodes:    []Node{{NodeID: article, Kind: "dieu", Path: "dieu-1", Text: "\t \n\u2003\n\r "}},
			file:     "f0eeb977d05fff4e200a71cd75a57fb3b768d327c9be0ac983a4ee8960f54b22",
			coverage: "46f9b216dba75b9b6462f7c82b87e522773c11aa780d4a20d75602993aeadb7e",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			recipe := Recipe2()
			recipe.BodyLimit, recipe.MinimumRunes = 12, 3
			file, coverage, err := Split(tc.nodes, Meta{Key: "vn:synthetic:1", Number: "1/TEST", Title: "Synthetic law"}, recipe)
			if err != nil {
				t.Fatal(err)
			}
			assertJSONHash(t, "file", file, tc.file)
			assertJSONHash(t, "coverage", coverage, tc.coverage)
		})
	}
}

func assertJSONHash(t *testing.T, name string, value any, want string) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(data)); got != want {
		t.Errorf("%s SHA-256 = %s, want %s; JSON: %s", name, got, want, data)
	}
}
