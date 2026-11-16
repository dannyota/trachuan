CREATE TABLE IF NOT EXISTS framework (
 id bigint PRIMARY KEY CONSTRAINT framework_id_check CHECK (id > 0),
 code text COLLATE "C" NOT NULL UNIQUE CONSTRAINT framework_code_check CHECK (code ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
 name text NOT NULL, publisher text NOT NULL, aliases text[] NOT NULL
);

CREATE TABLE IF NOT EXISTS framework_version (
 id bigint PRIMARY KEY CONSTRAINT framework_version_id_check CHECK (id > 0),
 framework_code text COLLATE "C" NOT NULL CONSTRAINT framework_version_framework_fkey REFERENCES framework(code),
 version text COLLATE "C" NOT NULL CONSTRAINT framework_version_version_check CHECK (version ~ '^[0-9a-z]+([.+-][0-9a-z]+)*$'),
 language text NOT NULL,
 access text NOT NULL CONSTRAINT framework_version_access_check CHECK (access IN ('free','licensed')),
 licence_kind text NOT NULL, licence_url text,
 released date NOT NULL, ordinal integer NOT NULL CONSTRAINT framework_version_ordinal_check CHECK (ordinal > 0),
 supersedes_id bigint, amends_id bigint,
 CONSTRAINT framework_version_code_version_key UNIQUE (framework_code,version),
 CONSTRAINT framework_version_order_key UNIQUE (framework_code,released,ordinal),
 UNIQUE (framework_code,id), UNIQUE (framework_code,version,id),
 CONSTRAINT framework_version_supersedes_fkey FOREIGN KEY (framework_code,supersedes_id) REFERENCES framework_version(framework_code,id),
 CONSTRAINT framework_version_amends_fkey FOREIGN KEY (framework_code,amends_id) REFERENCES framework_version(framework_code,id),
 CONSTRAINT framework_version_supersedes_check CHECK (supersedes_id IS NULL OR supersedes_id <> id),
 CONSTRAINT framework_version_amends_check CHECK (amends_id IS NULL OR amends_id <> id)
);

CREATE TABLE IF NOT EXISTS source_file (
 id bigint PRIMARY KEY CONSTRAINT source_file_id_check CHECK (id > 0),
 version_id bigint NOT NULL CONSTRAINT source_file_version_fkey REFERENCES framework_version(id),
 name text COLLATE "C" NOT NULL,
 role text NOT NULL CONSTRAINT source_file_role_check CHECK (role IN ('text','catalog','mapping')),
 sha256 text COLLATE "C" NOT NULL CONSTRAINT source_file_sha256_check CHECK (sha256 ~ '^[0-9a-f]{64}$'),
 size bigint NOT NULL CONSTRAINT source_file_size_check CHECK (size >= 0), licence_kind text NOT NULL,
 text_source text NOT NULL CONSTRAINT source_file_text_source_check CHECK (text_source IN ('structured','pdf_text','ocr')),
 text_complete boolean NOT NULL,
 CONSTRAINT source_file_identity_key UNIQUE (version_id,name,sha256)
);

CREATE TABLE IF NOT EXISTS control (
 id bigint PRIMARY KEY CONSTRAINT control_id_check CHECK (id > 0),
 version_id bigint NOT NULL REFERENCES framework_version(id), parent_id bigint,
 ordinal integer NOT NULL CONSTRAINT control_ordinal_check CHECK (ordinal > 0),
 citation text COLLATE "C" NOT NULL CONSTRAINT control_citation_check CHECK (
  octet_length(citation) BETWEEN 1 AND 64 AND citation ~ '^[!-~]+( [!-~]+)*$'
 ),
 citation_norm text COLLATE "C" NOT NULL CONSTRAINT control_citation_norm_check CHECK (citation_norm = upper(replace(citation,' ',''))),
 kind text NOT NULL, title text NOT NULL, status text NOT NULL,
 supersedes_id bigint CONSTRAINT control_supersedes_fkey REFERENCES control(id), amends_id bigint REFERENCES control(id),
 UNIQUE (version_id,id), CONSTRAINT control_version_citation_key UNIQUE (version_id,citation),
 CONSTRAINT control_version_citation_norm_key UNIQUE (version_id,citation_norm),
 CONSTRAINT control_sibling_ordinal_key UNIQUE NULLS NOT DISTINCT (version_id,parent_id,ordinal),
 CONSTRAINT control_parent_fkey FOREIGN KEY (version_id,parent_id) REFERENCES control(version_id,id),
 CONSTRAINT control_parent_check CHECK (parent_id IS NULL OR parent_id <> id),
 CONSTRAINT control_supersedes_check CHECK (supersedes_id IS NULL OR supersedes_id <> id),
 CONSTRAINT control_amends_check CHECK (amends_id IS NULL OR amends_id <> id)
);

CREATE TABLE IF NOT EXISTS control_text (
 control_id bigint PRIMARY KEY CONSTRAINT control_text_control_fkey REFERENCES control(id),
 title_original text NOT NULL, body text NOT NULL, notes text[] NOT NULL, tables text[] NOT NULL
);

CREATE TABLE IF NOT EXISTS body_chunk (
 id bigint PRIMARY KEY CONSTRAINT body_chunk_id_check CHECK (id > 0), version_id bigint NOT NULL,
 control_id bigint NOT NULL, ordinal integer NOT NULL CONSTRAINT body_chunk_ordinal_check CHECK (ordinal >= 0),
 content text NOT NULL CONSTRAINT body_chunk_content_check CHECK (content <> ''),
 input_sha256 text COLLATE "C" NOT NULL CONSTRAINT body_chunk_input_sha256_check CHECK (input_sha256 ~ '^[0-9a-f]{64}$'),
 token_count integer NOT NULL CONSTRAINT body_chunk_token_count_check CHECK (token_count > 0),
 embedding vector({{dimensions}}) NOT NULL,
 CONSTRAINT body_chunk_control_ordinal_key UNIQUE (control_id,ordinal), UNIQUE (version_id,id),
 CONSTRAINT body_chunk_control_fkey FOREIGN KEY (version_id,control_id) REFERENCES control(version_id,id)
);

CREATE TABLE IF NOT EXISTS public_chunk (
 id bigint PRIMARY KEY CONSTRAINT public_chunk_id_check CHECK (id > 0), version_id bigint NOT NULL,
 control_id bigint NOT NULL CONSTRAINT public_chunk_control_key UNIQUE,
 UNIQUE (version_id,id),
 content text NOT NULL CONSTRAINT public_chunk_content_check CHECK (content <> ''),
 input_sha256 text COLLATE "C" NOT NULL CONSTRAINT public_chunk_input_sha256_check CHECK (input_sha256 ~ '^[0-9a-f]{64}$'),
 token_count integer NOT NULL CONSTRAINT public_chunk_token_count_check CHECK (token_count > 0),
 embedding vector({{dimensions}}) NOT NULL,
 CONSTRAINT public_chunk_control_fkey FOREIGN KEY (version_id,control_id) REFERENCES control(version_id,id)
);

CREATE TABLE IF NOT EXISTS body_posting (
 version_id bigint NOT NULL, chunk_id bigint NOT NULL,
 token text COLLATE "C" NOT NULL CONSTRAINT body_posting_token_check CHECK (token <> ''),
 tf integer NOT NULL CONSTRAINT body_posting_tf_check CHECK (tf > 0), PRIMARY KEY (version_id,chunk_id,token),
 CONSTRAINT body_posting_chunk_fkey FOREIGN KEY (version_id,chunk_id) REFERENCES body_chunk(version_id,id)
);

CREATE TABLE IF NOT EXISTS public_posting (
 version_id bigint NOT NULL, chunk_id bigint NOT NULL,
 token text COLLATE "C" NOT NULL CONSTRAINT public_posting_token_check CHECK (token <> ''),
 tf integer NOT NULL CONSTRAINT public_posting_tf_check CHECK (tf > 0), PRIMARY KEY (version_id,chunk_id,token),
 CONSTRAINT public_posting_chunk_fkey FOREIGN KEY (version_id,chunk_id) REFERENCES public_chunk(version_id,id)
);

CREATE OR REPLACE VIEW body_lexical_stats AS
 SELECT version_id, count(*) AS chunk_count, avg(token_count) AS avg_length
 FROM body_chunk GROUP BY version_id;
CREATE OR REPLACE VIEW body_lexical_df AS
 SELECT version_id, token, count(*) AS df FROM body_posting GROUP BY version_id,token;
CREATE OR REPLACE VIEW public_lexical_stats AS
 SELECT count(*) AS chunk_count, coalesce(avg(token_count),0) AS avg_length FROM public_chunk;
CREATE OR REPLACE VIEW public_lexical_df AS
 SELECT token, count(*) AS df FROM public_posting GROUP BY token;

CREATE TABLE IF NOT EXISTS control_mapping (
 id bigint PRIMARY KEY CONSTRAINT control_mapping_id_check CHECK (id > 0),
 from_code text COLLATE "C" NOT NULL CONSTRAINT control_mapping_from_code_check CHECK (from_code ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
 from_version text COLLATE "C" NOT NULL CONSTRAINT control_mapping_from_version_check CHECK (from_version ~ '^[0-9a-z]+([.+-][0-9a-z]+)*$'),
 from_citation text COLLATE "C" NOT NULL CONSTRAINT control_mapping_from_citation_check CHECK (
  octet_length(from_citation) BETWEEN 1 AND 64 AND from_citation ~ '^[!-~]+( [!-~]+)*$'
 ),
 to_code text COLLATE "C" NOT NULL CONSTRAINT control_mapping_to_code_check CHECK (to_code ~ '^[a-z][a-z0-9]*(-[a-z0-9]+)*$'),
 to_version text COLLATE "C" NOT NULL CONSTRAINT control_mapping_to_version_check CHECK (to_version ~ '^[0-9a-z]+([.+-][0-9a-z]+)*$'),
 to_citation text COLLATE "C" NOT NULL CONSTRAINT control_mapping_to_citation_check CHECK (
  octet_length(to_citation) BETWEEN 1 AND 64 AND to_citation ~ '^[!-~]+( [!-~]+)*$'
 ),
 relation text COLLATE "C" NOT NULL CONSTRAINT control_mapping_relation_check CHECK (relation IN ('equivalent','subset','superset','intersects','related')),
 source text COLLATE "C" NOT NULL CONSTRAINT control_mapping_source_check CHECK (source <> ''),
 from_version_id bigint, to_version_id bigint, resolved boolean NOT NULL,
 CONSTRAINT control_mapping_tuple_key UNIQUE (from_code,from_version,from_citation,to_code,to_version,to_citation,relation,source),
 CONSTRAINT control_mapping_from_version_fkey FOREIGN KEY (from_code,from_version,from_version_id) REFERENCES framework_version(framework_code,version,id),
 CONSTRAINT control_mapping_to_version_fkey FOREIGN KEY (to_code,to_version,to_version_id) REFERENCES framework_version(framework_code,version,id),
 CONSTRAINT control_mapping_from_control_fkey FOREIGN KEY (from_version_id,from_citation) REFERENCES control(version_id,citation),
 CONSTRAINT control_mapping_to_control_fkey FOREIGN KEY (to_version_id,to_citation) REFERENCES control(version_id,citation),
 CONSTRAINT control_mapping_resolved_check CHECK (resolved = (from_version_id IS NOT NULL AND to_version_id IS NOT NULL)),
 CONSTRAINT control_mapping_versions_check CHECK (from_code <> to_code OR from_version <> to_version)
);

CREATE TABLE IF NOT EXISTS mapping_evidence (
 mapping_id bigint PRIMARY KEY CONSTRAINT mapping_evidence_mapping_fkey REFERENCES control_mapping(id),
 evidence text NOT NULL, licensed_version_ids bigint[] NOT NULL
);

CREATE TABLE IF NOT EXISTS quality_gap (
 kind text COLLATE "C" NOT NULL, class text COLLATE "C" NOT NULL,
 version_id bigint NOT NULL CONSTRAINT quality_gap_version_fkey REFERENCES framework_version(id),
 subject text COLLATE "C" NOT NULL,
 detail text COLLATE "C" NOT NULL CONSTRAINT quality_gap_detail_check CHECK (detail ~ '^(0|[1-9][0-9]*)(:(0|[1-9][0-9]*))*$'),
 blocks boolean NOT NULL,
 PRIMARY KEY (kind,version_id,subject,detail),
 CONSTRAINT quality_gap_class_check CHECK (
  (class='public' AND kind IN ('mapping_unresolved','mapping_dropped','title_missing')) OR
  (class='text' AND kind IN ('ocr_blocked','ocr_flagged','text_incomplete','chunk_omission','embedding_input_too_long','embedding_input_bytes_exceeded'))
 ),
 CONSTRAINT quality_gap_blocks_check CHECK (
  (kind <> 'ocr_blocked' OR blocks) AND (NOT blocks OR kind IN ('ocr_blocked','title_missing'))
 )
);

CREATE TABLE IF NOT EXISTS release (
 singleton boolean PRIMARY KEY CONSTRAINT release_singleton_check CHECK (singleton),
 kind text COLLATE "C" NOT NULL CONSTRAINT release_kind_check CHECK (kind = 'frameworks'),
 rel text COLLATE "C" NOT NULL UNIQUE,
 schema_version integer NOT NULL CONSTRAINT release_schema_version_check CHECK (schema_version = 1),
 dense_recipe jsonb NOT NULL CONSTRAINT release_dense_recipe_check CHECK (jsonb_typeof(dense_recipe) = 'object'),
 chunk_recipe jsonb NOT NULL CONSTRAINT release_chunk_recipe_check CHECK (jsonb_typeof(chunk_recipe) = 'object'),
 lexical_recipe jsonb NOT NULL CONSTRAINT release_lexical_recipe_check CHECK (jsonb_typeof(lexical_recipe) = 'object'),
 retrieval_recipe jsonb NOT NULL CONSTRAINT release_retrieval_recipe_check CHECK (jsonb_typeof(retrieval_recipe) = 'object'),
 snapshot_recipe jsonb NOT NULL CONSTRAINT release_snapshot_recipe_check CHECK (jsonb_typeof(snapshot_recipe) = 'object'),
 parse_recipe jsonb NOT NULL CONSTRAINT release_parse_recipe_check CHECK (jsonb_typeof(parse_recipe) = 'object'),
 relate_recipe jsonb NOT NULL CONSTRAINT release_relate_recipe_check CHECK (jsonb_typeof(relate_recipe) = 'object'),
 embed_recipe jsonb NOT NULL CONSTRAINT release_embed_recipe_check CHECK (jsonb_typeof(embed_recipe) = 'object'),
 snapshot_input_hash text COLLATE "C" NOT NULL CONSTRAINT release_snapshot_input_hash_check CHECK (snapshot_input_hash ~ '^[0-9a-f]{64}$'),
 parse_input_hash text COLLATE "C" NOT NULL CONSTRAINT release_parse_input_hash_check CHECK (parse_input_hash ~ '^[0-9a-f]{64}$'),
 relate_input_hash text COLLATE "C" NOT NULL CONSTRAINT release_relate_input_hash_check CHECK (relate_input_hash ~ '^[0-9a-f]{64}$'),
 chunk_input_hash text COLLATE "C" NOT NULL CONSTRAINT release_chunk_input_hash_check CHECK (chunk_input_hash ~ '^[0-9a-f]{64}$'),
 embed_input_hash text COLLATE "C" NOT NULL CONSTRAINT release_embed_input_hash_check CHECK (embed_input_hash ~ '^[0-9a-f]{64}$'),
 framework_count bigint NOT NULL CONSTRAINT release_framework_count_check CHECK (framework_count >= 0),
 version_count bigint NOT NULL CONSTRAINT release_version_count_check CHECK (version_count >= 0),
 control_count bigint NOT NULL CONSTRAINT release_control_count_check CHECK (control_count >= 0), built_at timestamptz NOT NULL,
 versions_sealed boolean NOT NULL DEFAULT false
);
