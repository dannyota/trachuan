"""Throwaway spike: load the parsed Bộ pháp điển package into schema spike."""

import os
import re
import sys
from pathlib import Path

import psycopg

from parse import parse_demuc, parse_tree

_ARTICLE_NO = re.compile(r"Dieu_(\w+)$")
_NOTE_ARTICLE = re.compile(r"\(Điều\s+(\d+[a-zđ]?)\b")
_DOC_NUMBER = re.compile(r"\bsố\s+(\d+/\d{4}/[0-9A-Za-zĐđ-]+)")

DDL = """
drop table if exists spike.phapdien_article, spike.phapdien_tree, spike.phapdien_demuc, spike.phapdien_chude;
create schema if not exists spike;
create table spike.phapdien_chude (id text primary key, stt text, name text);
create table spike.phapdien_demuc (id text primary key, chude_id text, stt text, name text);
create table spike.phapdien_tree (id text primary key, mapc text, demuc_id text, chude_id text, label text, name text);
create table spike.phapdien_article (seq int primary key, mapc text, demuc_id text, label text, doc_kind text, item_id text,
  anchor text, article_no text, doc_number text, source_url text, source_note text, title text);
"""


def main(root: Path):
    chude, demuc, tree = parse_tree(root / "jsonData.js")
    articles = [a for f in sorted((root / "demuc").glob("*.html")) for a in parse_demuc(f)]
    with psycopg.connect(os.environ["DATABASE_URL"]) as conn, conn.cursor() as cur:
        cur.execute(DDL)
        with cur.copy("copy spike.phapdien_chude from stdin") as cp:
            for c in chude:
                cp.write_row((c.id, c.stt, c.name))
        with cur.copy("copy spike.phapdien_demuc from stdin") as cp:
            for d in demuc:
                cp.write_row((d.id, d.chude_id, d.stt, d.name))
        with cur.copy("copy spike.phapdien_tree from stdin") as cp:
            for n in tree:
                cp.write_row((n.id, n.mapc, n.demuc_id, n.chude_id, n.label, n.name))
        with cur.copy("copy spike.phapdien_article from stdin") as cp:
            for seq, a in enumerate(articles, start=1):
                no = _ARTICLE_NO.search(a.anchor) or _NOTE_ARTICLE.search(a.source_note)
                num = _DOC_NUMBER.search(a.source_note)
                cp.write_row((seq, a.mapc, a.demuc_id, a.label, a.doc_kind, a.item_id or None, a.anchor or None,
                              no.group(1) if no else None, num.group(1) if num else None,
                              a.source_url or None, a.source_note, a.title))
        conn.commit()
        cur.execute("select (select count(*) from spike.phapdien_chude), (select count(*) from spike.phapdien_demuc),"
                    " (select count(*) from spike.phapdien_tree), (select count(*) from spike.phapdien_article),"
                    " (select count(*) from spike.phapdien_article where article_no is null),"
                    " (select count(*) from spike.phapdien_article where doc_number is null)")
        print("chude, demuc, tree, articles, no article_no, no doc_number:", cur.fetchone())


if __name__ == "__main__":
    main(Path(sys.argv[1] if len(sys.argv) > 1 else "../../work/phapdien/pkg"))
