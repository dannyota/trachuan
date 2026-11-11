// Package taxonomy reads the Pháp điển taxonomy package.
package taxonomy

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Package contains the taxonomy and article references from a package.
type Package struct {
	SHA256   string
	ChuDe    []ChuDe
	DeMuc    []DeMuc
	Tree     []TreeNode
	Articles []Article
}

// ChuDe is a subject.
type ChuDe struct {
	GUID string
	STT  int
	Name string
}

// DeMuc is a subject's section.
type DeMuc struct {
	GUID, ChuDeGUID string
	STT             int
	Name            string
	Removed         bool
}

// NodeKind identifies a tree node's structural kind.
type NodeKind string

// Tree node kinds.
const (
	KindPhan    NodeKind = "phan"
	KindChuong  NodeKind = "chuong"
	KindMuc     NodeKind = "muc"
	KindTieuMuc NodeKind = "tieu_muc"
	KindDieu    NodeKind = "dieu"
)

// TreeNode is a node in the package tree.
type TreeNode struct {
	GUID, MAPC, DeMucGUID, ParentGUID string
	Kind                              NodeKind
	Name                              string
	Order                             int
}

// Article identifies a placed article without retaining its text.
type Article struct {
	DeMucGUID, MAPC          string
	Seq                      int
	ItemID, Number, TreeGUID string
}

const maxEntry = 64 << 20

// ReadPackage reads a zip package without extracting its contents.
func ReadPackage(zipPath string) (Package, error) {
	data, err := os.ReadFile(zipPath)
	if err != nil {
		return Package{}, fmt.Errorf("read package: %w", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return Package{}, fmt.Errorf("open package: %w", err)
	}
	p := Package{SHA256: fmt.Sprintf("%x", sha256.Sum256(data))}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		name := f.Name
		if name != "jsonData.js" && (!strings.HasPrefix(name, "demuc/") || !strings.HasSuffix(name, ".html") || strings.Contains(strings.TrimPrefix(name, "demuc/"), "/")) {
			continue
		}
		if _, ok := files[name]; ok {
			return Package{}, fmt.Errorf("duplicate entry %s", name)
		}
		files[name] = f
	}
	f, ok := files["jsonData.js"]
	if !ok {
		return Package{}, fmt.Errorf("missing jsonData.js")
	}
	raw, err := readEntry(f)
	if err != nil {
		return Package{}, err
	}
	if err = parseJSON(raw, &p); err != nil {
		return Package{}, err
	}
	first := make(map[string]string, len(p.Tree))
	for _, n := range p.Tree {
		if _, ok := first[n.MAPC]; !ok {
			first[n.MAPC] = n.GUID
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		if name != "jsonData.js" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		raw, err = readEntry(files[name])
		if err != nil {
			return Package{}, err
		}
		id := strings.ToLower(strings.TrimSuffix(strings.TrimPrefix(name, "demuc/"), ".html"))
		p.Articles = appendArticles(p.Articles, raw, id, first)
	}
	return p, nil
}

func readEntry(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > maxEntry {
		return nil, fmt.Errorf("entry %s exceeds 64 MB", f.Name)
	}
	r, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", f.Name, err)
	}
	defer func() { _ = r.Close() }()
	b, err := io.ReadAll(io.LimitReader(r, maxEntry+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", f.Name, err)
	}
	if len(b) > maxEntry {
		return nil, fmt.Errorf("entry %s exceeds 64 MB", f.Name)
	}
	return b, nil
}

func array(raw []byte, name string, dst any) error {
	re := regexp.MustCompile(`\bvar\s+` + name + `\s*=\s*`)
	loc := re.FindIndex(raw)
	if loc == nil {
		return fmt.Errorf("missing %s", name)
	}
	if err := json.NewDecoder(bytes.NewReader(raw[loc[1]:])).Decode(dst); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	return nil
}
func parseJSON(raw []byte, p *Package) error {
	var subjects []struct{ Value, Text, STT string }
	var sections []struct{ Value, ChuDe, Text, STT string }
	var nodes []struct{ ID, MAPC, DeMucID, TEN string }
	if err := array(raw, "jdChuDe", &subjects); err != nil {
		return err
	}
	if err := array(raw, "jdDeMuc", &sections); err != nil {
		return err
	}
	if err := array(raw, "jdAllTree", &nodes); err != nil {
		return err
	}
	for _, v := range subjects {
		n, err := strconv.Atoi(v.STT)
		if err != nil {
			return fmt.Errorf("subject %s STT: %w", v.Value, err)
		}
		p.ChuDe = append(p.ChuDe, ChuDe{strings.ToLower(v.Value), n, v.Text})
	}
	for _, v := range sections {
		n, err := strconv.Atoi(v.STT)
		if err != nil {
			return fmt.Errorf("section %s STT: %w", v.Value, err)
		}
		const suffix = " (loại bỏ)"
		p.DeMuc = append(p.DeMuc, DeMuc{strings.ToLower(v.Value), strings.ToLower(v.ChuDe), n, strings.TrimSuffix(v.Text, suffix), strings.HasSuffix(v.Text, suffix)})
	}
	for _, v := range nodes {
		kind, err := nodeKind(v.TEN)
		if err != nil {
			return fmt.Errorf("node %s: %w", strings.ToLower(v.ID), err)
		}
		p.Tree = append(p.Tree, TreeNode{GUID: strings.ToLower(v.ID), MAPC: v.MAPC, DeMucGUID: strings.ToLower(v.DeMucID), Kind: kind, Name: v.TEN})
	}
	setParentsAndOrder(p.Tree)
	return nil
}
func nodeKind(name string) (NodeKind, error) {
	for _, v := range []struct {
		prefix string
		kind   NodeKind
	}{{"Tiểu mục", KindTieuMuc}, {"Phần", KindPhan}, {"Chương", KindChuong}, {"Mục", KindMuc}, {"Điều", KindDieu}} {
		if name == v.prefix || strings.HasPrefix(name, v.prefix+" ") {
			return v.kind, nil
		}
	}
	return "", fmt.Errorf("unknown node kind in %q", name)
}
func setParentsAndOrder(nodes []TreeNode) {
	type key struct{ section, code string }
	first := make(map[key]string, len(nodes))
	for _, n := range nodes {
		k := key{n.DeMucGUID, n.MAPC}
		if _, ok := first[k]; !ok {
			first[k] = n.GUID
		}
	}
	groups := map[key][]int{}
	for i := range nodes {
		n := &nodes[i]
		if len(n.MAPC) > 20 {
			n.ParentGUID = first[key{n.DeMucGUID, n.MAPC[:len(n.MAPC)-20]}]
		}
		k := key{n.DeMucGUID, n.ParentGUID}
		groups[k] = append(groups[k], i)
	}
	for _, idx := range groups {
		sort.SliceStable(idx, func(i, j int) bool { return nodes[idx[i]].MAPC < nodes[idx[j]].MAPC })
		for order, i := range idx {
			nodes[i].Order = order + 1
		}
	}
}
