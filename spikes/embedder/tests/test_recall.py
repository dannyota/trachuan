"""Tests for the spike's recall metrics and banhmi-compatible citation matching."""

from recall import Hit, matches, mrr_at_k, recall_at_k


def test_recall_at_8_counts_hits():
    ranked = {
        "case-a": [Hit("50/2024/TT-NHNN", "Điều 3, Khoản 1")],
        "case-b": [Hit("13/2023/NĐ-CP", "Điều 2")],
    }
    expected = {
        "case-a": [{"doc_number": "50/2024/tt-nhnn", "article": "3"}],
        "case-b": [{"doc_number": "91/2025/QH15", "article": "20"}],
    }
    assert recall_at_k(ranked, expected, 8) == 0.5
    assert mrr_at_k(ranked, expected, 8) == 0.5


def test_recall_ignores_hits_beyond_k():
    hits = [Hit("x/2020/TT-NHNN", f"Điều {i}") for i in range(1, 9)] + [Hit("50/2024/TT-NHNN", "Điều 3")]
    ranked = {"c": hits}
    expected = {"c": [{"doc_number": "50/2024/TT-NHNN", "article": "3"}]}
    assert recall_at_k(ranked, expected, 8) == 0.0
    assert recall_at_k(ranked, expected, 9) == 1.0
    assert mrr_at_k(ranked, expected, 9) == 1 / 9


def test_matches_requires_same_article_token_not_prefix():
    expected = {"doc_number": "50/2024/TT-NHNN", "article": "3"}
    assert matches(expected, Hit("50/2024/TT-NHNN", "Điều 3, Khoản 2"))
    assert not matches(expected, Hit("50/2024/TT-NHNN", "Điều 30"))
    assert not matches(expected, Hit("51/2024/TT-NHNN", "Điều 3"))


def test_matches_clause_and_point_when_given():
    expected = {"doc_number": "50/2024/TT-NHNN", "article": "3", "clause": "2", "point": "a"}
    assert matches(expected, Hit("50/2024/TT-NHNN", "Điều 3, Khoản 2, Điểm a"))
    assert not matches(expected, Hit("50/2024/TT-NHNN", "Điều 3, Khoản 1"))


def test_recall_is_micro_averaged_over_expected_citations():
    ranked = {"c": [Hit("A/2020/TT", "Điều 1")]}
    expected = {"c": [{"doc_number": "A/2020/TT", "article": "1"}, {"doc_number": "A/2020/TT", "article": "2"}],
                "abstain-case": []}
    assert recall_at_k(ranked, expected, 8) == 0.5
