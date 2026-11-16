//go:build integration

package frameworks

const releaseFixtureSQL = `INSERT INTO release(
 singleton,kind,rel,schema_version,dense_recipe,chunk_recipe,lexical_recipe,retrieval_recipe,
 snapshot_recipe,parse_recipe,relate_recipe,embed_recipe,
 snapshot_input_hash,parse_input_hash,relate_input_hash,chunk_input_hash,embed_input_hash,
 framework_count,version_count,control_count,built_at)
 VALUES(true,'frameworks','20990101',1,'{}','{}','{}','{}','{}','{}','{}','{}',
 repeat('a',64),repeat('b',64),repeat('c',64),repeat('d',64),repeat('e',64),2,2,2,now());`
