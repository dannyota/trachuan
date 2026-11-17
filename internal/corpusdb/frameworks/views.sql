CREATE OR REPLACE VIEW body_lexical_stats AS
 SELECT version_id, count(*) AS chunk_count, avg(token_count) AS avg_length
 FROM body_chunk GROUP BY version_id;
CREATE OR REPLACE VIEW body_lexical_df AS
 SELECT version_id, token, count(*) AS df FROM body_posting GROUP BY version_id,token;
CREATE OR REPLACE VIEW public_lexical_stats AS
 SELECT count(*) AS chunk_count, coalesce(avg(token_count),0) AS avg_length FROM public_chunk;
CREATE OR REPLACE VIEW public_lexical_df AS
 SELECT token, count(*) AS df FROM public_posting GROUP BY token;

