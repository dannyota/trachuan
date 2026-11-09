"""Throwaway spike: dense ranking and recall/MRR with banhmi's golden matching rule.

Matching rule (from banhmi pkg/eval/eval.go, rewritten, not copied): a hit matches
an expected citation when its document number agrees with the expected number or
one of its alt_doc_numbers after upper-casing and dropping every character
outside [A-Za-z0-9] (equality or containment), and, for
each of article, clause and point the expectation names, the hit citation has a
token equal to the keyword ("điều", "khoản", "điểm") followed by a token equal to
the wanted value after trimming parentheses and dots. Recall is micro-averaged
over expected citations; MRR is the mean reciprocal rank over cases that expect
at least one citation. Not applied: banhmi's relation_ok credit (a hit whose
confirmed relations name the expected document); dense chunk hits carry no
relations here. Two of the 96 golden citations use it.
"""

import re
from dataclasses import dataclass

import numpy as np

KEYWORDS = {"article": "điều", "clause": "khoản", "point": "điểm"}
_NON_ALNUM = re.compile(r"[^A-Za-z0-9]")


@dataclass(frozen=True)
class Hit:
    doc_number: str
    citation: str
    chunk_id: int = 0


def _doc_norm(s: str) -> str:
    return _NON_ALNUM.sub("", s.upper())


def _same_doc(a: str, b: str) -> bool:
    if a.strip().casefold() == b.strip().casefold():
        return True
    na, nb = _doc_norm(a), _doc_norm(b)
    return na == nb or na in nb or nb in na


def _citation_has(citation: str, keyword: str, want: str) -> bool:
    tokens = [t for t in re.split(r"[,\s]+", citation) if t]
    for i in range(len(tokens) - 1):
        if tokens[i].casefold() == keyword and tokens[i + 1].strip("().").casefold() == want.casefold():
            return True
    return False


def matches(expected: dict, hit: Hit) -> bool:
    """Report whether hit satisfies one expected citation from golden.json."""
    numbers = [expected["doc_number"], *expected.get("alt_doc_numbers", [])]
    if not any(_same_doc(hit.doc_number or "", n) for n in numbers):
        return False
    return all(
        _citation_has(hit.citation, kw, expected[field])
        for field, kw in KEYWORDS.items()
        if expected.get(field)
    )


def recall_at_k(ranked: dict[str, list[Hit]], expected: dict[str, list[dict]], k: int) -> float:
    """Micro-averaged recall@k over all expected citations."""
    found = want = 0
    for case, citations in expected.items():
        hits = ranked.get(case, [])[:k]
        for ec in citations:
            want += 1
            found += any(matches(ec, h) for h in hits)
    return found / want if want else 0.0


def mrr_at_k(ranked: dict[str, list[Hit]], expected: dict[str, list[dict]], k: int) -> float:
    """Mean reciprocal rank of the first matching hit, over cases with expectations."""
    total, cases = 0.0, 0
    for case, citations in expected.items():
        if not citations:
            continue
        cases += 1
        for rank, h in enumerate(ranked.get(case, [])[:k], start=1):
            if any(matches(ec, h) for ec in citations):
                total += 1.0 / rank
                break
    return total / cases if cases else 0.0


def dense_rank(conn, query_vectors: dict[str, np.ndarray], table: str, k: int,
               subset: str | None = None) -> dict[str, list[Hit]]:
    """Rank chunks by exact cosine distance in `table` (chunk_id, embedding) for each query,
    optionally only among chunk ids listed in the `subset` table."""
    out: dict[str, list[Hit]] = {}
    with conn.cursor() as cur:
        cur.execute("set enable_indexscan = off")
        for case, vec in query_vectors.items():
            literal = "[" + ",".join(f"{x:.7f}" for x in vec.astype(np.float32)) + "]"
            cur.execute(
                f"select c.id, coalesce(d.doc_number, ''), c.citation from {table} e "
                + (f"join {subset} s on s.chunk_id = e.chunk_id " if subset else "")
                + "join gold.chunk c on c.id = e.chunk_id join silver.document d on d.id = c.document_id "
                "order by e.embedding <=> %s::vector limit %s",
                (literal, k),
            )
            out[case] = [Hit(doc, cit, cid) for cid, doc, cit in cur.fetchall()]
    return out
