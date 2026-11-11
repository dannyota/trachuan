package taxonomy

import (
	"strings"
	"testing"
)

func TestImport_CodeRules(t *testing.T) {
	p := Package{SHA256: "hash", ChuDe: []ChuDe{{GUID: "s", STT: 22, Name: " Subject "}}, DeMuc: []DeMuc{{GUID: "d", ChuDeGUID: "s", STT: 3, Name: "Section"}, {GUID: "dup", ChuDeGUID: "s", STT: 3, Name: "Other"}, {GUID: "old", ChuDeGUID: "s", STT: 7, Name: "Removed", Removed: true}}, Tree: []TreeNode{
		{GUID: "c", DeMucGUID: "d", Kind: KindChuong, Name: "Chapter", Order: 1}, {GUID: "m", DeMucGUID: "d", ParentGUID: "c", Kind: KindMuc, Name: "Section", Order: 1}, {GUID: "t", DeMucGUID: "d", ParentGUID: "m", Kind: KindTieuMuc, Name: "Subsection", Order: 1}, {GUID: "a", DeMucGUID: "d", ParentGUID: "t", Kind: KindDieu, Name: "Article", Order: 1}, {GUID: "guide", DeMucGUID: "d", ParentGUID: "a", Kind: KindDieu, Name: "Guide", Order: 1}, {GUID: "bare", DeMucGUID: "d", Kind: KindDieu, Name: "Article", Order: 2}, {GUID: "orphanm", DeMucGUID: "d", Kind: KindMuc, Name: "Orphan", Order: 3}, {GUID: "orphant", DeMucGUID: "d", Kind: KindTieuMuc, Name: "Orphan", Order: 4},
		{GUID: "p", DeMucGUID: "dup", Kind: KindPhan, Name: "Part", Order: 1}, {GUID: "pc", DeMucGUID: "dup", ParentGUID: "p", Kind: KindChuong, Name: "Chapter", Order: 1}, {GUID: "pm", DeMucGUID: "dup", ParentGUID: "pc", Kind: KindMuc, Name: "Section", Order: 1},
	}}
	m, err := Import(p, Mapping{})
	if err != nil {
		t.Fatal(err)
	}
	nodes := map[string]MappedNode{}
	for _, n := range m.Nodes {
		nodes[n.GUID] = n
	}
	for id, code := range map[string]string{"s": "22", "d": "22.3", "dup": "22.4", "old": "22.7", "c": "22.3.c1", "m": "22.3.c1.m1", "p": "22.4.p1", "pc": "22.4.p1.c1"} {
		if nodes[id].Code != code {
			t.Errorf("%s code = %q, want %q", id, nodes[id].Code, code)
		}
	}
	if nodes["old"].Status != "retired" || nodes["dup"].Status != "active" {
		t.Fatal("wrong statuses", nodes)
	}
	flat := map[string]Flattened{}
	for _, f := range m.Flattened {
		flat[f.GUID] = f
	}
	for id, want := range map[string]Flattened{"t": {GUID: "t", Into: "22.3.c1.m1", Reason: "tieu_muc"}, "pm": {GUID: "pm", Into: "22.4.p1.c1", Reason: "muc_under_level4_chuong"}, "orphanm": {GUID: "orphanm", Into: "22.3", Reason: "no_structural_parent"}, "orphant": {GUID: "orphant", Into: "22.3", Reason: "no_structural_parent"}} {
		if flat[id] != want {
			t.Errorf("%s = %+v", id, flat[id])
		}
	}
	for id, want := range map[string]string{"guide": "22.3.c1.m1", "a": "22.3.c1.m1", "bare": "22.3"} {
		got, ok := NewArticleResolver(p, m).Code(Article{DeMucGUID: "d", TreeGUID: id})
		if !ok || got != want {
			t.Errorf("article %s: %q %v", id, got, ok)
		}
	}
}

func TestImport_StableAndRetired(t *testing.T) {
	p := Package{ChuDe: []ChuDe{{GUID: "s", STT: 1, Name: "S"}}, DeMuc: []DeMuc{{GUID: "d", ChuDeGUID: "s", STT: 1, Name: "D"}}, Tree: []TreeNode{{GUID: "a", DeMucGUID: "d", Kind: KindChuong, Name: "A", Order: 1}, {GUID: "b", DeMucGUID: "d", Kind: KindChuong, Name: "B", Order: 2}}}
	first, err := Import(p, Mapping{})
	if err != nil {
		t.Fatal(err)
	}
	p.Tree = []TreeNode{{GUID: "b", DeMucGUID: "d", Kind: KindChuong, Name: "B", Order: 1}, {GUID: "new", DeMucGUID: "d", Kind: KindChuong, Name: "New", Order: 2}}
	second, err := Import(p, first)
	if err != nil {
		t.Fatal(err)
	}
	nodes := map[string]MappedNode{}
	for _, n := range second.Nodes {
		nodes[n.GUID] = n
	}
	if nodes["a"].Code != "1.1.c1" || nodes["a"].Status != "retired" || nodes["a"].Ordinal != 0 || nodes["b"].Code != "1.1.c2" || nodes["b"].Ordinal != 1 || nodes["new"].Code != "1.1.c3" {
		t.Fatal(nodes)
	}
}

func TestEncode_NumericOrder(t *testing.T) {
	m := Mapping{Nodes: []MappedNode{{Code: "22.10"}, {Code: "22.3"}}}
	s := string(m.Encode())
	if strings.Index(s, "22.3") > strings.Index(s, "22.10") {
		t.Fatal(s)
	}
}

func TestImport_ReorderAndArticleRetired(t *testing.T) {
	p := Package{ChuDe: []ChuDe{{GUID: "s", STT: 1, Name: "S"}}, DeMuc: []DeMuc{{GUID: "d", ChuDeGUID: "s", STT: 1, Name: "D"}}, Tree: []TreeNode{{GUID: "a", DeMucGUID: "d", Kind: KindChuong, Name: "A", Order: 1}, {GUID: "b", DeMucGUID: "d", Kind: KindChuong, Name: "B", Order: 2}}}
	first, err := Import(p, Mapping{})
	if err != nil {
		t.Fatal(err)
	}
	p.Tree[0].Order = 2
	p.Tree[1].Order = 1
	second, err := Import(p, first)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range second.Nodes {
		if n.GUID == "a" && (n.Code != "1.1.c1" || n.Ordinal != 2) {
			t.Fatal(n)
		}
		if n.GUID == "b" && (n.Code != "1.1.c2" || n.Ordinal != 1) {
			t.Fatal(n)
		}
	}
	p.DeMuc[0].Removed = true
	third, err := Import(p, second)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := NewArticleResolver(p, third).Code(Article{DeMucGUID: "d", TreeGUID: "a"}); ok {
		t.Fatal("retired section resolved")
	}
	if _, ok := NewArticleResolver(p, second).Code(Article{DeMucGUID: "d"}); ok {
		t.Fatal("unplaced article resolved")
	}
}

func TestImport_StructureTransitions(t *testing.T) {
	p := Package{ChuDe: []ChuDe{{GUID: "s", STT: 22, Name: "Subject"}}, DeMuc: []DeMuc{{GUID: "d", ChuDeGUID: "s", STT: 3, Name: "Section"}}, Tree: []TreeNode{{GUID: "c", DeMucGUID: "d", Kind: KindChuong, Name: "Chapter", Order: 1}, {GUID: "m", DeMucGUID: "d", ParentGUID: "c", Kind: KindMuc, Name: "Muc", Order: 1}}}
	a, err := Import(p, Mapping{})
	if err != nil {
		t.Fatal(err)
	}
	p.Tree = append(p.Tree, TreeNode{GUID: "p", DeMucGUID: "d", Kind: KindPhan, Name: "Part", Order: 1})
	p.Tree[0].ParentGUID = "p"
	b, err := Import(p, a)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range b.Nodes {
		if n.GUID == "c" && (n.Code != "22.3.c1" || n.ParentCode != "22.3.p1" || n.Level != 4) {
			t.Fatalf("moved chapter: %+v", n)
		}
		if n.GUID == "m" && (n.Code != "22.3.c1.m1" || n.Status != "retired" || n.Level != 4 || n.ParentCode != "22.3.c1") {
			t.Fatalf("retired muc: %+v", n)
		}
	}
	retiredMuc := false
	for _, n := range b.Nodes {
		if n.GUID == "m" && n.Status == "retired" && n.Code == "22.3.c1.m1" {
			retiredMuc = true
		}
	}
	if !retiredMuc {
		t.Fatal("missing retired muc")
	}
	found := false
	for _, f := range b.Flattened {
		if f.GUID == "m" && f.Into == "22.3.c1" && f.Reason == "muc_under_level4_chuong" {
			found = true
		}
	}
	if !found {
		t.Fatal("missing flattened muc")
	}
	if err = Validate(b, nil, nil); err != nil {
		t.Fatal(err)
	}
	p.Tree[0].ParentGUID = ""
	c, err := Import(p, b)
	if err != nil {
		t.Fatal(err)
	}
	active, retired := false, false
	for _, n := range c.Nodes {
		if n.GUID == "m" && n.Status == "active" && n.Code == "22.3.c1.m2" {
			active = true
		}
		if n.GUID == "m" && n.Status == "retired" && n.Code == "22.3.c1.m1" {
			retired = true
		}
	}
	if !active || !retired {
		t.Fatalf("transition: %+v", c.Nodes)
	}
	if err = Validate(c, nil, nil); err != nil {
		t.Fatal(err)
	}
}

func TestImport_NewSiblingAfterMovedNode(t *testing.T) {
	p := Package{ChuDe: []ChuDe{{GUID: "s", STT: 1, Name: "S"}}, DeMuc: []DeMuc{{GUID: "d", ChuDeGUID: "s", STT: 1, Name: "D"}}, Tree: []TreeNode{{GUID: "a", DeMucGUID: "d", Kind: KindChuong, Name: "A", Order: 1}, {GUID: "b", DeMucGUID: "d", Kind: KindChuong, Name: "B", Order: 2}}}
	a, err := Import(p, Mapping{})
	if err != nil {
		t.Fatal(err)
	}
	p.Tree = append(p.Tree, TreeNode{GUID: "p", DeMucGUID: "d", Kind: KindPhan, Name: "P", Order: 3})
	p.Tree[1].ParentGUID = "p"
	b, err := Import(p, a)
	if err != nil {
		t.Fatal(err)
	}
	p.Tree = append(p.Tree, TreeNode{GUID: "c", DeMucGUID: "d", ParentGUID: "p", Kind: KindChuong, Name: "C", Order: 2})
	c, err := Import(p, b)
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range c.Nodes {
		if n.GUID == "c" && n.Code != "1.1.p1.c1" {
			t.Fatalf("new sibling: %+v", n)
		}
	}
}

func TestImport_MovedCodeNeverReused(t *testing.T) {
	p := Package{ChuDe: []ChuDe{{GUID: "s", STT: 1, Name: "S"}}, DeMuc: []DeMuc{{GUID: "d", ChuDeGUID: "s", STT: 1, Name: "D"}}, Tree: []TreeNode{{GUID: "a", DeMucGUID: "d", Kind: KindChuong, Name: "A", Order: 1}}}
	a, err := Import(p, Mapping{})
	if err != nil {
		t.Fatal(err)
	}
	p.Tree = append(p.Tree, TreeNode{GUID: "p", DeMucGUID: "d", Kind: KindPhan, Name: "P", Order: 1})
	p.Tree[0].ParentGUID = "p"
	b, err := Import(p, a)
	if err != nil {
		t.Fatal(err)
	}
	p.Tree = append(p.Tree, TreeNode{GUID: "new", DeMucGUID: "d", Kind: KindChuong, Name: "New", Order: 2})
	c, err := Import(p, b)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, n := range c.Nodes {
		if n.GUID == "new" {
			found = true
			if n.Code != "1.1.c2" {
				t.Fatalf("new code %s", n.Code)
			}
		}
	}
	if !found {
		t.Fatal("new chapter missing")
	}
}

func TestImport_SubjectCollision(t *testing.T) {
	p := Package{ChuDe: []ChuDe{{GUID: "a", STT: 22, Name: "A"}, {GUID: "b", STT: 22, Name: "B"}}}
	m, err := Import(p, Mapping{})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Nodes) != 2 || m.Nodes[0].Code != "22" || m.Nodes[1].Code != "23" {
		t.Fatalf("nodes %+v", m.Nodes)
	}
}
