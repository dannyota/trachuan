CREATE TABLE document (
 id bigint PRIMARY KEY CHECK (id > 0),
 country text COLLATE "C" NOT NULL CHECK (country = 'vn'),
 key text COLLATE "C" NOT NULL UNIQUE CHECK (key LIKE 'vn:%'),
 number text NOT NULL, number_norm text COLLATE "C" NOT NULL,
 title text NOT NULL, doc_type text NOT NULL, issuer text NOT NULL,
 class text NOT NULL CHECK (class IN ('primary','relation_context','secondary')),
 source text NOT NULL CHECK (source IN ('vbpl','chinhphu')),
 issue_date date, eff_from date, eff_to date,
 raw_issue_date text NOT NULL, raw_eff_from text NOT NULL, raw_eff_to text NOT NULL,
 status_code text NOT NULL, status_name text NOT NULL,
 status_class text NOT NULL CHECK (status_class IN ('in_force','partial','expired','not_yet','suspended','unknown')),
 status_basis text NOT NULL CHECK (status_basis IN ('source_status','derived_from_consolidated','derived_from_text','manual_correction')),
 validity_conflict boolean NOT NULL,
 is_current boolean NOT NULL CHECK (is_current = (status_class IN ('in_force','partial'))),
 is_consolidated boolean NOT NULL,
 content_state text NOT NULL CHECK (content_state IN ('indexed','no_tree','no_chunks','embedding_gap')),
 fetch_sha256 text COLLATE "C" NOT NULL CHECK (fetch_sha256 ~ '^[0-9a-f]{64}$'),
 extract_sha256 text COLLATE "C" CHECK (extract_sha256 ~ '^[0-9a-f]{64}$'),
 source_url text,
 CHECK (eff_to IS NULL OR eff_from IS NULL OR eff_to >= eff_from)
);
CREATE INDEX ON document (number_norm);
CREATE INDEX ON document (status_class, eff_from, eff_to);

CREATE TABLE source_file (
 id bigint PRIMARY KEY CHECK (id > 0), document_id bigint NOT NULL REFERENCES document(id),
 source text NOT NULL, name text COLLATE "C" NOT NULL, url text,
 hash text COLLATE "C" NOT NULL CHECK (hash ~ '^[0-9a-f]{64}$'),
 size bigint NOT NULL CHECK (size >= 0),
 role text NOT NULL CHECK (role IN ('main','appendix','scan','unknown')),
 fetched_at timestamptz, UNIQUE (document_id, name, hash)
);
CREATE INDEX ON source_file (document_id);

CREATE TABLE provision (
 id bigint PRIMARY KEY CHECK (id > 0), document_id bigint NOT NULL REFERENCES document(id),
 parent_id bigint, node_id text COLLATE "C" NOT NULL,
 kind text NOT NULL CHECK (kind IN ('phan','chuong','muc','dieu','khoan','diem','other')),
 label text NOT NULL, heading text NOT NULL, ordinal integer NOT NULL CHECK (ordinal > 0),
 preorder integer NOT NULL CHECK (preorder > 0), path ltree NOT NULL, text text NOT NULL,
 anchor text COLLATE "C" NOT NULL,
 UNIQUE (document_id,id), UNIQUE (document_id,node_id), UNIQUE (document_id,anchor),
 UNIQUE (document_id,preorder), UNIQUE (document_id,path),
 FOREIGN KEY (document_id,parent_id) REFERENCES provision (document_id,id),
 CHECK (parent_id IS NULL OR parent_id <> id)
);
CREATE INDEX ON provision (document_id,parent_id,ordinal);
CREATE INDEX ON provision USING gist (path);

CREATE TABLE provision_validity (
 id bigint PRIMARY KEY CHECK (id > 0), provision_id bigint NOT NULL REFERENCES provision(id),
 eff_from date, eff_to date,
 status_class text NOT NULL CHECK (status_class IN ('in_force','partial','expired','not_yet','suspended','unknown')),
 evidence text NOT NULL CHECK (evidence <> ''), source_url text NOT NULL,
 method text NOT NULL, observed_at timestamptz NOT NULL,
 CHECK (eff_from IS NOT NULL OR eff_to IS NOT NULL),
 CHECK (eff_to IS NULL OR eff_from IS NULL OR eff_to >= eff_from),
 UNIQUE (provision_id,observed_at)
);
CREATE INDEX ON provision_validity (provision_id,eff_from,eff_to);

CREATE TABLE chunk (
 id bigint PRIMARY KEY CHECK (id > 0), document_id bigint NOT NULL REFERENCES document(id),
 provision_id bigint NOT NULL, ordinal integer NOT NULL CHECK (ordinal >= 0),
 context_prefix text NOT NULL, content text NOT NULL CHECK (content <> ''),
 spans jsonb NOT NULL CHECK (jsonb_typeof(spans) = 'array'),
 input_sha256 text COLLATE "C" NOT NULL CHECK (input_sha256 ~ '^[0-9a-f]{64}$'),
 token_count integer NOT NULL CHECK (token_count > 0),
 embedding vector({{dimensions}}) NOT NULL, sparse sparsevec(1048576) NOT NULL,
 UNIQUE (document_id,ordinal),
 FOREIGN KEY (document_id,provision_id) REFERENCES provision (document_id,id)
);
CREATE INDEX ON chunk (provision_id);

CREATE TABLE lexical_df (
 token text COLLATE "C" PRIMARY KEY, dimension integer NOT NULL CHECK (dimension BETWEEN 1 AND 1048576),
 df bigint NOT NULL CHECK (df > 0)
);

CREATE TABLE relation (
 id bigint PRIMARY KEY CHECK (id > 0), source_document_id bigint REFERENCES document(id),
 target_document_id bigint REFERENCES document(id), source_key text COLLATE "C" NOT NULL,
 target_key text COLLATE "C", source_number text NOT NULL, target_number text NOT NULL,
 kind text NOT NULL CHECK (kind IN ('repeals','implements','legal_basis','cites','suspends','consolidates','guided_by','amends_supplements','replaces','references','unknown')),
 raw_code text NOT NULL, raw_label text NOT NULL, source_provision_id bigint, target_provision_id bigint,
 source_node_id text COLLATE "C", target_node_id text COLLATE "C", evidence text NOT NULL, method text NOT NULL,
 artifact_sha256 text COLLATE "C" NOT NULL CHECK (artifact_sha256 ~ '^[0-9a-f]{64}$'),
 ordinal integer NOT NULL CHECK (ordinal >= 0), UNIQUE (source_key,artifact_sha256,ordinal),
 FOREIGN KEY (source_document_id,source_provision_id) REFERENCES provision (document_id,id),
 FOREIGN KEY (target_document_id,target_provision_id) REFERENCES provision (document_id,id),
 CHECK (source_provision_id IS NULL OR source_document_id IS NOT NULL),
 CHECK (target_provision_id IS NULL OR target_document_id IS NOT NULL),
 CHECK (target_key IS NOT NULL OR target_number <> '' OR kind = 'unknown')
);
CREATE INDEX ON relation (source_document_id);
CREATE INDEX ON relation (target_document_id);
CREATE INDEX ON relation (target_key);

CREATE TABLE validity_decision (
 document_id bigint NOT NULL REFERENCES document(id),
 ordinal smallint NOT NULL CHECK (ordinal BETWEEN 1 AND 8),
 layer text NOT NULL CHECK (layer IN ('source','A','B','C','manual')),
 rule_version text NOT NULL, before jsonb NOT NULL, after jsonb NOT NULL,
 actor_key text COLLATE "C", effect_date date, relation_id bigint REFERENCES relation(id),
 anchor text COLLATE "C", quote text CHECK (char_length(quote) <= 400),
 artifact_sha256 text COLLATE "C" CHECK (artifact_sha256 ~ '^[0-9a-f]{64}$'),
 PRIMARY KEY (document_id,ordinal),
 CHECK (layer NOT IN ('B','C') OR (actor_key IS NOT NULL AND effect_date IS NOT NULL)),
 CHECK (layer <> 'C' OR relation_id IS NOT NULL)
);

CREATE TABLE taxonomy_node (
 id bigint PRIMARY KEY CHECK (id > 0), country text COLLATE "C" NOT NULL CHECK (country = 'vn'),
 parent_id bigint REFERENCES taxonomy_node(id), level integer NOT NULL CHECK (level BETWEEN 1 AND 4),
 code text COLLATE "C" NOT NULL UNIQUE, name text NOT NULL, description text NOT NULL,
 origin text NOT NULL CHECK (origin IN ('phapdien','custom')),
 status text NOT NULL CHECK (status IN ('active','retired')), path ltree NOT NULL UNIQUE,
 CHECK (parent_id IS NULL OR parent_id <> id)
);
CREATE INDEX ON taxonomy_node (parent_id,code);
CREATE INDEX ON taxonomy_node USING gist (path);

CREATE TABLE coverage_gap (
 id bigint PRIMARY KEY CHECK (id > 0), kind text COLLATE "C" NOT NULL,
 document_id bigint REFERENCES document(id), document_key text COLLATE "C", node_id bigint REFERENCES taxonomy_node(id),
 stage text COLLATE "C" NOT NULL, item_key text COLLATE "C" NOT NULL,
 detail text COLLATE "C" NOT NULL, message text NOT NULL, accepted_at timestamptz, note text NOT NULL,
 UNIQUE (stage,item_key,kind,detail)
);
CREATE INDEX ON coverage_gap (kind,id);
CREATE INDEX ON coverage_gap (document_id);
CREATE INDEX ON coverage_gap (node_id);

CREATE TABLE taxonomy_assignment (
 provision_id bigint NOT NULL REFERENCES provision(id), node_id bigint NOT NULL REFERENCES taxonomy_node(id),
 family_id bigint NOT NULL REFERENCES taxonomy_node(id),
 method text NOT NULL CHECK (method IN ('manual','phapdien_seed','llm')), exclude boolean NOT NULL,
 confidence double precision NOT NULL CHECK (confidence BETWEEN 0 AND 1),
 taxonomy_version text COLLATE "C" NOT NULL CHECK (taxonomy_version <> ''),
 PRIMARY KEY (provision_id,node_id), CHECK (NOT exclude OR method = 'manual')
);
CREATE INDEX ON taxonomy_assignment (node_id,provision_id);
CREATE INDEX ON taxonomy_assignment (provision_id,family_id);

CREATE TABLE release (
 singleton boolean PRIMARY KEY CHECK (singleton), country text COLLATE "C" NOT NULL CHECK (country = 'vn'),
 rel text COLLATE "C" NOT NULL UNIQUE, schema_version integer NOT NULL CHECK (schema_version = 3),
 taxonomy_version text COLLATE "C" NOT NULL,
 dense_recipe jsonb NOT NULL CHECK (jsonb_typeof(dense_recipe) = 'object'),
 chunk_recipe jsonb NOT NULL CHECK (jsonb_typeof(chunk_recipe) = 'object'),
 lexical_recipe jsonb NOT NULL CHECK (jsonb_typeof(lexical_recipe) = 'object'),
 retrieval_recipe jsonb NOT NULL CHECK (jsonb_typeof(retrieval_recipe) = 'object'),
 lexical_stats jsonb NOT NULL CHECK (jsonb_typeof(lexical_stats) = 'object'),
 source_manifest_hash text COLLATE "C" NOT NULL CHECK (source_manifest_hash ~ '^[0-9a-f]{64}$'),
 build_input_hash text COLLATE "C" NOT NULL CHECK (build_input_hash ~ '^[0-9a-f]{64}$'),
 build_recipe_hash text COLLATE "C" NOT NULL CHECK (build_recipe_hash ~ '^[0-9a-f]{64}$'),
 evaluation_summary jsonb NOT NULL CHECK (jsonb_typeof(evaluation_summary) = 'object'),
 built_at timestamptz NOT NULL
);
