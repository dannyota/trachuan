"""Throwaway spike: coverage report for the Bộ pháp điển join, as Markdown tables."""

import os

import psycopg

QUERIES = {
    "Package articles by reason": """
        select reason, coalesce(method, '-'), count(*), round(100.0 * count(*) / sum(count(*)) over (), 1)
        from spike.phapdien_join group by 1, 2 order by 3 desc""",
    "Corpus articles (Điều) with at least one seed label": """
        with dieu as (
          select ds.id, d.index_class, d.is_consolidated from silver.document_section ds
          join silver.document d on d.id = ds.document_id where ds.kind = 'dieu'),
        seeded as (select distinct section_id from spike.phapdien_join where section_id is not null)
        select case when is_consolidated then 'consolidated' else index_class end, count(*),
               count(s.section_id), round(100.0 * count(s.section_id) / count(*), 1)
        from dieu left join seeded s on s.section_id = dieu.id group by 1 order by 2 desc""",
    "Corpus documents with at least one seeded article": """
        with docs as (
          select d.id, case when d.is_consolidated then 'consolidated' else d.index_class end as cls
          from silver.document d where exists (select 1 from silver.document_section s where s.document_id = d.id and s.kind = 'dieu')),
        seeded as (select distinct ds.document_id from spike.phapdien_join j
                   join silver.document_section ds on ds.id = j.section_id)
        select cls, count(*), count(s.document_id), round(100.0 * count(s.document_id) / count(*), 1)
        from docs left join seeded s on s.document_id = docs.id group by 1 order by 2 desc""",
    "Package articles in chủ đề 22 (Ngân hàng, tiền tệ), 34 (Thương mại, đầu tư, chứng khoán), 1 (An ninh quốc gia)": """
        select chude_stt, reason, count(*) from spike.phapdien_join
        where chude_stt in ('22', '34', '1') group by 1, 2 order by 1, 3 desc""",
    "Corpus primary articles seeded, by chủ đề of the seed (top 12)": """
        select j.chude_stt, c.name, count(distinct j.section_id) from spike.phapdien_join j
        join spike.phapdien_chude c on c.stt = j.chude_stt
        join silver.document_section ds on ds.id = j.section_id join silver.document d on d.id = ds.document_id
        where d.index_class = 'primary' and not d.is_consolidated group by 1, 2 order by 3 desc limit 12""",
    "Corpus articles seeded into two or more đề mục": """
        select n_demuc, count(*) from (select section_id, count(distinct demuc_id) n_demuc from spike.phapdien_join
        where section_id is not null group by 1) t group by 1 order by 1""",
    "Consolidated-text articles that could inherit a seed through `consolidates`": """
        with seeded as (select distinct document_id, article_no from spike.phapdien_join where reason = 'matched_provision'),
        cons as (select distinct x.from_document_id vbhn, r.document_id orig from silver.document_relation x
                 join silver.doc_ref r on r.id = x.to_ref_id where x.relation_type = 'consolidates' and r.document_id is not null),
        vbhn_art as (select ds.id, ds.document_id, substring(ds.citation_path from 'dieu-([0-9]+[a-z]?)$') no
                     from silver.document_section ds join silver.document d on d.id = ds.document_id
                     where d.is_consolidated and ds.kind = 'dieu')
        select (select count(*) from vbhn_art), count(distinct v.id) from vbhn_art v
        join cons c on c.vbhn = v.document_id join seeded s on s.document_id = c.orig and s.article_no = v.no""",
    "Package keys: MAPC uniqueness": """
        select count(*), count(distinct mapc), count(distinct (demuc_id, label)) from spike.phapdien_article""",
}


def main():
    with psycopg.connect(os.environ["DATABASE_URL"]) as conn:
        for title, sql in QUERIES.items():
            rows = conn.execute(sql).fetchall()
            print(f"\n### {title}\n")
            for r in rows:
                print("| " + " | ".join(str(x) for x in r) + " |")


if __name__ == "__main__":
    main()
