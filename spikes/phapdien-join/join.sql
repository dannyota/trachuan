-- Throwaway spike: classify every Bộ pháp điển article against banhmi's Vietnam corpus.
-- Document: VBPL ItemID alias first (silver.document_alias source 'vbpl'), then the
-- document number in the source note (silver.document.doc_number, case and space folded).
-- Article: silver.document_section kind 'dieu' whose citation_path ends in 'dieu-<n>'.
-- Reasons: matched_provision, document_matched_article_missing, consolidated_only,
-- document_not_in_corpus.
drop table if exists spike.phapdien_join;
create table spike.phapdien_join as
with art as (
  select a.*, d.stt as demuc_stt, c.stt as chude_stt,
         upper(replace(a.doc_number, ' ', '')) as num_key
  from spike.phapdien_article a
  join spike.phapdien_demuc d on d.id = a.demuc_id
  join spike.phapdien_chude c on c.id = d.chude_id
),
by_item as (
  select a.seq, al.document_id from art a
  join silver.document_alias al on al.source = 'vbpl' and al.external_id = a.item_id
),
by_number as (
  select a.seq, min(doc.id) as document_id from art a
  join silver.document doc on upper(replace(doc.doc_number, ' ', '')) = a.num_key and not doc.is_consolidated
  where a.num_key is not null
  group by 1
),
by_consolidation as (
  select a.seq, min(x.from_document_id) as document_id from art a
  join silver.doc_ref r on (r.ref_key = 'vbpl:' || a.item_id or upper(replace(r.label, ' ', '')) = a.num_key)
  join silver.document_relation x on x.to_ref_id = r.id and x.relation_type = 'consolidates'
  group by 1
),
resolved as (
  select a.seq, a.mapc, a.demuc_id, a.chude_stt, a.demuc_stt, a.doc_kind, a.article_no,
         coalesce(bi.document_id, bn.document_id, bc.document_id) as document_id,
         case when bi.document_id is not null then 'item_id'
              when bn.document_id is not null then 'doc_number'
              when bc.document_id is not null then 'consolidation' end as method
  from art a
  left join by_item bi using (seq)
  left join by_number bn using (seq)
  left join by_consolidation bc using (seq)
)
select r.*, s.id as section_id,
       case when r.document_id is null then 'document_not_in_corpus'
            when r.method = 'consolidation' then 'consolidated_only'
            when s.id is null then 'document_matched_article_missing'
            else 'matched_provision' end as reason
from resolved r
left join lateral (
  select ds.id from silver.document_section ds
  where ds.document_id = r.document_id and ds.kind = 'dieu'
    and ds.citation_path ~ ('(^|/)dieu-' || r.article_no || '$')
  order by ds.id limit 1
) s on r.article_no is not null;
create index on spike.phapdien_join (section_id);
select reason, method, count(*) from spike.phapdien_join group by 1, 2 order by 3 desc;
