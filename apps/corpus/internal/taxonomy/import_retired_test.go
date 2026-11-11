package taxonomy

import (
	"bytes"
	"testing"
)

func TestImport_RetiredCodeNeverReturns(t *testing.T) {
	base := Package{ChuDe: []ChuDe{{GUID: "s", STT: 1, Name: "S"}}, DeMuc: []DeMuc{{GUID: "d", ChuDeGUID: "s", STT: 1, Name: "D"}}, Tree: []TreeNode{{GUID: "c", DeMucGUID: "d", Kind: KindChuong, Name: "C", Order: 1}, {GUID: "m", DeMucGUID: "d", ParentGUID: "c", Kind: KindMuc, Name: "M", Order: 1}}}
	run := func(p Package, prev Mapping) Mapping {
		t.Helper()
		m, err := Import(p, prev)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	node := func(m Mapping, id, status string) MappedNode {
		t.Helper()
		for _, n := range m.Nodes {
			if n.GUID == id && n.Status == status {
				return n
			}
		}
		t.Fatalf("missing %s %s", id, status)
		return MappedNode{}
	}
	t.Run("flattened then missing", func(t *testing.T) {
		p := base
		p.Tree = append([]TreeNode(nil), base.Tree...)
		a := run(p, Mapping{})
		p.Tree = append(p.Tree, TreeNode{GUID: "p", DeMucGUID: "d", Kind: KindPhan, Name: "P", Order: 2})
		p.Tree[0].ParentGUID = "p"
		b := run(p, a)
		p.Tree = p.Tree[:1]
		p.Tree[0].ParentGUID = ""
		c := run(p, b)
		p.Tree = append(p.Tree, base.Tree[1])
		d := run(p, c)
		if got := node(d, "m", "active").Code; got != "1.1.c1.m2" {
			t.Fatalf("reactivated %s", got)
		}
	})
	t.Run("missing then active", func(t *testing.T) {
		p := base
		p.Tree = append([]TreeNode(nil), base.Tree...)
		a := run(p, Mapping{})
		p.Tree = p.Tree[:1]
		b := run(p, a)
		p.Tree = append(p.Tree, base.Tree[1])
		c := run(p, b)
		if got := node(c, "m", "active").Code; got != "1.1.c1.m2" {
			t.Fatalf("reactivated %s", got)
		}
	})
	t.Run("removed then active", func(t *testing.T) {
		p := base
		p.DeMuc = append([]DeMuc(nil), base.DeMuc...)
		a := run(p, Mapping{})
		p.DeMuc[0].Removed = true
		b := run(p, a)
		p.DeMuc[0].Removed = false
		c := run(p, b)
		if got := node(c, "d", "active").Code; got != "1.2" {
			t.Fatalf("reactivated section %s", got)
		}
		if got := node(c, "c", "active").Code; got != "1.2.c1" {
			t.Fatalf("reactivated chapter %s", got)
		}
	})
	t.Run("stays removed", func(t *testing.T) {
		p := base
		p.DeMuc = append([]DeMuc(nil), base.DeMuc...)
		active := run(p, Mapping{})
		p.DeMuc[0].Removed = true
		removed := run(p, active)
		again := run(p, removed)
		if len(removed.Flattened) != 0 || len(again.Flattened) != 0 {
			t.Fatal("removed section has flattened entries")
		}
		for _, id := range []string{"d", "c", "m"} {
			before, after := node(removed, id, "retired"), node(again, id, "retired")
			if before != after {
				t.Fatalf("%s changed: %+v to %+v", id, before, after)
			}
		}
		if !bytes.Equal(removed.Encode(), again.Encode()) {
			t.Fatal("removed import changed mapping")
		}
		firstRemoved := run(p, Mapping{})
		secondRemoved := run(p, firstRemoved)
		if !bytes.Equal(firstRemoved.Encode(), secondRemoved.Encode()) || len(firstRemoved.Nodes) != 2 || len(firstRemoved.Flattened) != 0 {
			t.Fatal("new removed section generated subtree nodes")
		}
	})
}

func TestImport_RemovedKeepsMostRecentCode(t *testing.T) {
	p := Package{ChuDe: []ChuDe{{GUID: "s1", STT: 1, Name: "S1"}, {GUID: "s2", STT: 2, Name: "S2"}}, DeMuc: []DeMuc{{GUID: "d", ChuDeGUID: "s2", STT: 1, Name: "D"}}}
	run := func(prev Mapping) Mapping {
		t.Helper()
		m, err := Import(p, prev)
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	a := run(Mapping{})
	p.DeMuc[0].Removed = true
	b := run(a)
	p.DeMuc[0].Removed = false
	p.DeMuc[0].ChuDeGUID = "s1"
	c := run(b)
	p.DeMuc[0].Removed = true
	d := run(c)
	e := run(d)
	for _, want := range []struct{ code, parent string }{{"2.1", "2"}, {"1.1", "1"}} {
		found := false
		for _, n := range e.Nodes {
			if n.GUID == "d" && n.Code == want.code && n.ParentCode == want.parent && n.Status == "retired" {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing retired code %s with parent %s", want.code, want.parent)
		}
	}
	if !bytes.Equal(d.Encode(), e.Encode()) {
		t.Fatal("retired history changed")
	}
}
