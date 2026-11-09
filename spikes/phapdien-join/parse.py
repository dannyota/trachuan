"""Throwaway spike: parse the 2023 Bộ pháp điển offline package (BoPhapDienDienTu.zip).

Each demuc/<GUID>.html lists placed articles as
<p class='pDieu'><a name='<MAPC>'></a>Điều 36.3.LQ.1. Title</p> followed by
<p class='pGhiChu'><a href='...vbpq-toanvan.aspx?ItemID=<id>#<anchor>'>(source note)</a></p>.
jsonData.js holds the arrays jdChuDe, jdDeMuc and jdAllTree.
"""

import html
import json
import re
from dataclasses import dataclass
from pathlib import Path


@dataclass(frozen=True)
class Article:
    demuc_id: str
    mapc: str
    label: str
    doc_kind: str
    item_id: str
    anchor: str
    source_url: str
    source_note: str = ""
    title: str = ""


@dataclass(frozen=True)
class ChuDe:
    id: str
    stt: str
    name: str


@dataclass(frozen=True)
class DeMuc:
    id: str
    chude_id: str
    stt: str
    name: str


@dataclass(frozen=True)
class TreeNode:
    id: str
    mapc: str
    demuc_id: str
    chude_id: str
    label: str
    name: str


_ARTICLE = re.compile(
    r"<p class='pDieu'><a name='(?P<mapc>\d+)'></a>(?P<text>.*?)</p>"
    r"(?:\s*<p class='pGhiChu'>(?P<note>.*?)</p>)?",
    re.S,
)
_LABEL = re.compile(r"^(?P<label>Điều\s+\d+\.\d+\.(?P<kind>[^.\s]+)(?:\.\d+)+)\.?\s*(?P<title>.*)$", re.S)
_HREF = re.compile(r"href='(?P<url>[^']*?ItemID=(?P<item>\d+)(?:#(?P<anchor>[^']*))?)'")
_TAGS = re.compile(r"<[^>]+>")
_DEMUC_ID = re.compile(r"ViewDanhSachVB\(this,'(?P<id>[0-9a-f-]{36})'\)")


def _text(fragment: str) -> str:
    return " ".join(html.unescape(_TAGS.sub(" ", fragment)).split())


def parse_demuc(path: Path) -> list[Article]:
    """Return every placed article in one đề mục file, in document order."""
    raw = Path(path).read_text(encoding="utf-8-sig")
    own = _DEMUC_ID.search(raw)
    demuc_id = own["id"] if own else Path(path).stem
    out = []
    for m in _ARTICLE.finditer(raw):
        label = _LABEL.match(_text(m["text"]))
        if not label:
            continue
        note = m["note"] or ""
        href = _HREF.search(note)
        out.append(Article(
            demuc_id=demuc_id,
            mapc=m["mapc"],
            label=label["label"],
            doc_kind=label["kind"],
            item_id=href["item"] if href else "",
            anchor=(href["anchor"] or "") if href else "",
            source_url=html.unescape(href["url"]) if href else "",
            source_note=_text(note),
            title=label["title"],
        ))
    return out


def _array(source: str, name: str) -> list[dict]:
    m = re.search(rf"var\s+{name}\s*=\s*(\[.*?\]);?\s*(?=var\s|\Z)", source, re.S)
    if not m:
        raise ValueError(f"{name} not found")
    return json.loads(m.group(1))


def parse_tree(json_data_js: Path) -> tuple[list[ChuDe], list[DeMuc], list[TreeNode]]:
    """Read the chủ đề, đề mục and full tree arrays from jsonData.js."""
    src = Path(json_data_js).read_text(encoding="utf-8-sig")
    chude = [ChuDe(c["Value"], c["STT"], c["Text"]) for c in _array(src, "jdChuDe")]
    demuc = [DeMuc(d["Value"], d["ChuDe"], d["STT"], d["Text"]) for d in _array(src, "jdDeMuc")]
    tree = [TreeNode(n["ID"], n["MAPC"], n["DeMucID"], n["ChuDeID"], n.get("ChiMuc") or "", n["TEN"])
            for n in _array(src, "jdAllTree")]
    return chude, demuc, tree


if __name__ == "__main__":
    import sys
    from collections import Counter

    root = Path(sys.argv[1] if len(sys.argv) > 1 else "../../work/phapdien/pkg")
    arts = [a for f in sorted((root / "demuc").glob("*.html")) for a in parse_demuc(f)]
    print("files", len(list((root / "demuc").glob("*.html"))), "articles", len(arts),
          "with item_id", sum(bool(a.item_id) for a in arts), "distinct item_ids", len({a.item_id for a in arts if a.item_id}))
    print("kinds", Counter(a.doc_kind for a in arts).most_common(12))
    print("anchor forms", Counter(re.sub(r"[0-9IVXLCa-z]+", "#", a.anchor) for a in arts).most_common(8))
