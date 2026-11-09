-- Throwaway spike: check the restored banhmi Vietnam corpus.
-- Expected (banhmi PLAN.md, 2026-08-14): documents 3974, chunks 142086, embeddings 142086.
\pset footer off
select 'documents' as item, count(*) as n from silver.document
union all select 'chunks', count(*) from gold.chunk
union all select 'embeddings', count(*) from gold.chunk_embedding;
select version() as server, (select extversion from pg_extension where extname = 'vector') as pgvector;
-- HNSW sanity: a stored vector's nearest neighbor is its own chunk.
set hnsw.ef_search = 100;
with probe as (select chunk_id, embedding from gold.chunk_embedding order by chunk_id limit 1 offset 5000)
select p.chunk_id as probe_chunk,
       (select e.chunk_id from gold.chunk_embedding e order by e.embedding <=> p.embedding limit 1) as nearest_chunk,
       (select count(*) from (select 1 from gold.chunk_embedding e order by e.embedding <=> p.embedding limit 10) t) as top10_rows
from probe p;
explain (costs off) select chunk_id from gold.chunk_embedding order by embedding <=> (select embedding from gold.chunk_embedding limit 1) limit 10;
