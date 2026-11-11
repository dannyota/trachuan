package taxonomy

import "testing"

func TestArticleResolver(t *testing.T) {
	p := Package{Tree: []TreeNode{{GUID: "c", DeMucGUID: "d", Kind: KindChuong}, {GUID: "m", DeMucGUID: "d", ParentGUID: "c", Kind: KindMuc}, {GUID: "t", DeMucGUID: "d", ParentGUID: "m", Kind: KindTieuMuc}, {GUID: "a", DeMucGUID: "d", ParentGUID: "t", Kind: KindDieu}, {GUID: "guide", DeMucGUID: "d", ParentGUID: "a", Kind: KindDieu}, {GUID: "bare", DeMucGUID: "d", Kind: KindDieu}, {GUID: "gone", DeMucGUID: "d", Kind: KindChuong}, {GUID: "under", DeMucGUID: "d", ParentGUID: "gone", Kind: KindDieu}, {GUID: "removed", DeMucGUID: "r", Kind: KindDieu}}}
	m := Mapping{Nodes: []MappedNode{{GUID: "d", Code: "1.1", Status: "active"}, {GUID: "r", Code: "1.2", Status: "retired"}, {GUID: "c", Code: "1.1.c1", Status: "active"}, {GUID: "m", Code: "1.1.c1.m1", Status: "active"}, {GUID: "m", Code: "1.1.c1.m2", Status: "retired"}, {GUID: "gone", Code: "1.1.c2", Status: "retired"}}, Flattened: []Flattened{{GUID: "t", Into: "1.1.c1.m1"}}}
	resolver := NewArticleResolver(p, m)
	for _, tt := range []struct {
		id, demuc, want string
		ok              bool
	}{{"guide", "d", "1.1.c1.m1", true}, {"a", "d", "1.1.c1.m1", true}, {"bare", "d", "1.1", true}, {"under", "d", "", false}, {"removed", "r", "", false}, {"", "d", "", false}, {"missing", "d", "", false}} {
		t.Run(tt.id+tt.demuc, func(t *testing.T) {
			got, ok := resolver.Code(Article{TreeGUID: tt.id, DeMucGUID: tt.demuc})
			if got != tt.want || ok != tt.ok {
				t.Fatalf("got %q %v, want %q %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}
