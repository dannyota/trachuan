package taxonomy

import "testing"

func TestImport_PackageOrdinals(t *testing.T) {
	p := Package{ChuDe: []ChuDe{{GUID: "s", STT: 1, Name: "S"}}, DeMuc: []DeMuc{{GUID: "d", ChuDeGUID: "s", STT: 1, Name: "D"}}, Tree: []TreeNode{{GUID: "a", DeMucGUID: "d", Kind: KindChuong, Name: "A", Order: 1}, {GUID: "b", DeMucGUID: "d", Kind: KindChuong, Name: "B", Order: 2}, {GUID: "c", DeMucGUID: "d", Kind: KindChuong, Name: "C", Order: 3}}}
	first, err := Import(p, Mapping{})
	if err != nil {
		t.Fatal(err)
	}
	p.Tree = []TreeNode{{GUID: "c", DeMucGUID: "d", Kind: KindChuong, Name: "C", Order: 1}, {GUID: "a", DeMucGUID: "d", Kind: KindChuong, Name: "A", Order: 2}}
	m, err := Import(p, first)
	if err != nil {
		t.Fatal(err)
	}
	if err = Validate(m, nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, n := range m.Nodes {
		switch n.GUID {
		case "a":
			if n.Ordinal != 2 {
				t.Fatal(n)
			}
		case "c":
			if n.Ordinal != 1 {
				t.Fatal(n)
			}
		case "b":
			if n.Status != "retired" || n.Ordinal != 0 {
				t.Fatal(n)
			}
		}
	}
	for _, tt := range []struct {
		name    string
		ordinal int
	}{{"gap", 3}, {"duplicate", 1}} {
		t.Run(tt.name, func(t *testing.T) {
			bad := m
			bad.Nodes = append([]MappedNode(nil), m.Nodes...)
			for i := range bad.Nodes {
				if bad.Nodes[i].GUID == "a" {
					bad.Nodes[i].Ordinal = tt.ordinal
				}
			}
			if err := Validate(bad, nil, nil); err == nil {
				t.Fatal("accepted invalid ordinals")
			}
		})
	}
}
