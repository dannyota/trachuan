"""Tests for parsing the 2023 Bộ pháp điển offline package."""

from pathlib import Path

from parse import parse_demuc, parse_tree

FIXTURE = Path(__file__).parent / "fixtures" / "36-3.html"


def test_parse_demuc_yields_articles():
    articles = parse_demuc(FIXTURE)
    assert articles[0].label == "Điều 36.3.LQ.1"
    assert articles[0].item_id == "142850"
    assert articles[0].anchor == "Chuong_I_Dieu_1"
    assert articles[0].doc_kind == "LQ"
    assert articles[0].demuc_id == "012d9b93-c11d-48b6-b08e-14d77515ca1e"
    assert all(a.mapc for a in articles)


def test_parse_demuc_reads_decree_articles_and_their_source():
    articles = parse_demuc(FIXTURE)
    decree = next(a for a in articles if a.label == "Điều 36.3.NĐ.1.1")
    assert decree.doc_kind == "NĐ"
    assert decree.item_id == "26146"
    assert decree.anchor == "Chuong_I_Dieu_1"
    assert "12/2011/NĐ-CP" in decree.source_note


def test_parse_tree_reads_the_three_arrays(tmp_path):
    js = tmp_path / "jsonData.js"
    js.write_text(
        '﻿var jdChuDe = [{"Value":"c1","Text":"Ngân hàng, tiền tệ","STT":"22"}];'
        'var jdDeMuc = [{"Value":"d1","Text":"Các tổ chức tín dụng","ChuDe":"c1","STT":"3"}];'
        'var jdAllTree = [{"ID":"n1","ChiMuc":"I","MAPC":"22003000000000001000","TEN":"Chương I Những quy định chung","ChuDeID":"c1","DeMucID":"d1"}];',
        encoding="utf-8",
    )
    chude, demuc, tree = parse_tree(js)
    assert (chude[0].id, chude[0].stt, chude[0].name) == ("c1", "22", "Ngân hàng, tiền tệ")
    assert (demuc[0].id, demuc[0].chude_id, demuc[0].stt) == ("d1", "c1", "3")
    assert (tree[0].id, tree[0].mapc, tree[0].demuc_id, tree[0].label) == ("n1", "22003000000000001000", "d1", "I")
