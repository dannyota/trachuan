//go:build integration

package frameworks

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
)

func publicProjection(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	var projection string
	err := conn.QueryRow(context.Background(), `SELECT jsonb_build_object(
 'controls',(SELECT jsonb_agg(to_jsonb(c) ORDER BY id) FROM control c),
 'chunks',(SELECT jsonb_agg(to_jsonb(c) ORDER BY id) FROM public_chunk c),
 'postings',(SELECT jsonb_agg(to_jsonb(p) ORDER BY chunk_id,token) FROM public_posting p),
 'stats',(SELECT to_jsonb(s) FROM public_lexical_stats s),
 'df',(SELECT jsonb_agg(to_jsonb(s) ORDER BY token) FROM public_lexical_df s),
 'edges',(SELECT jsonb_agg(to_jsonb(e) ORDER BY from_code,from_version,from_citation,to_code,to_version,to_citation,relation,source) FROM control_mapping e),
 'evidence',(SELECT jsonb_agg(to_jsonb(e) ORDER BY mapping_id) FROM mapping_evidence e WHERE licensed_version_ids <@ '{}'::bigint[]),
 'gaps',(SELECT jsonb_agg(to_jsonb(g) ORDER BY kind,version_id,subject,detail) FROM quality_gap g WHERE class='public')
 )::text`).Scan(&projection)
	if err != nil {
		t.Fatal(err)
	}
	return projection
}

func TestSchema_PublicProjectionIndependentOfText(t *testing.T) {
	conn := schemaDB(t)
	seed(t, conn)
	ctx := context.Background()
	before := publicProjection(t, conn)
	_, err := conn.Exec(ctx, `
 UPDATE control_text SET title_original='Changed invented title',body='Changed invented body',notes='{Invented}',tables='{Invented}';
 UPDATE body_chunk SET content='Changed invented chunk',token_count=9,embedding=array_fill(1::real,ARRAY[1024])::vector;
 INSERT INTO body_chunk(id,version_id,control_id,ordinal,content,input_sha256,token_count,embedding)
 VALUES(2,1,1,1,'Invented second chunk',repeat('c',64),3,array_fill(1::real,ARRAY[1024])::vector);
 INSERT INTO body_posting(version_id,chunk_id,token,tf) VALUES(1,2,'changed',2);
 UPDATE mapping_evidence SET evidence='Changed invented evidence';
 UPDATE source_file SET size=999,text_source='ocr',text_complete=false;
 INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks) VALUES('ocr_flagged','text',1,'A.1','1:2',false);`)
	if err != nil {
		t.Fatal(err)
	}
	if after := publicProjection(t, conn); before != after {
		t.Fatal("text changes altered the public projection")
	}
}

func TestSchema_LexicalStatisticsStayInTheirOwnIndex(t *testing.T) {
	conn := schemaDB(t)
	seed(t, conn)
	ctx := context.Background()
	_, err := conn.Exec(ctx, `
 INSERT INTO body_chunk(id,version_id,control_id,ordinal,content,input_sha256,token_count,embedding)
 VALUES(2,2,2,0,'Invented beta body',repeat('b',64),3,array_fill(0::real,ARRAY[1024])::vector);
 INSERT INTO body_posting(version_id,chunk_id,token,tf) VALUES(2,2,'invented',1);`)
	if err != nil {
		t.Fatal(err)
	}
	for _, phase := range []string{"before", "after"} {
		var count, df int
		var avg float64
		if err := conn.QueryRow(ctx, `SELECT chunk_count,avg_length::double precision FROM body_lexical_stats WHERE version_id=2`).Scan(&count, &avg); err != nil || count != 1 || avg != 3 {
			t.Fatalf("%s beta stats: %d %g %v", phase, count, avg, err)
		}
		if err := conn.QueryRow(ctx, `SELECT df FROM body_lexical_df WHERE version_id=2 AND token='invented'`).Scan(&df); err != nil || df != 1 {
			t.Fatalf("%s beta df: %d %v", phase, df, err)
		}
		if err := conn.QueryRow(ctx, `SELECT chunk_count,avg_length::double precision FROM public_lexical_stats`).Scan(&count, &avg); err != nil || count != 1 || avg != 5 {
			t.Fatalf("%s public stats: %d %g %v", phase, count, avg, err)
		}
		if err := conn.QueryRow(ctx, `SELECT df FROM public_lexical_df WHERE token='alpha'`).Scan(&df); err != nil || df != 1 {
			t.Fatalf("%s public df: %d %v", phase, df, err)
		}
		if phase == "before" {
			_, err := conn.Exec(ctx, `UPDATE body_chunk SET token_count=99 WHERE version_id=1;
    INSERT INTO body_chunk(id,version_id,control_id,ordinal,content,input_sha256,token_count,embedding)
    VALUES(3,1,1,1,'Invented alpha body',repeat('c',64),7,array_fill(0::real,ARRAY[1024])::vector);
    INSERT INTO body_posting(version_id,chunk_id,token,tf) VALUES(1,3,'invented',1);`)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	var count, df int
	var avg float64
	if err := conn.QueryRow(ctx, `SELECT chunk_count,avg_length::double precision FROM body_lexical_stats WHERE version_id=1`).Scan(&count, &avg); err != nil || count != 2 || avg != 53 {
		t.Fatalf("alpha stats: %d %g %v", count, avg, err)
	}
	if err := conn.QueryRow(ctx, `SELECT df FROM body_lexical_df WHERE version_id=1 AND token='invented'`).Scan(&df); err != nil || df != 2 {
		t.Fatalf("alpha df: %d %v", df, err)
	}
}

func TestSchema_AcceptsPublicEvidenceAndUnresolvedEnds(t *testing.T) {
	conn := schemaDB(t)
	seed(t, conn)
	_, err := conn.Exec(context.Background(), `
 UPDATE mapping_evidence SET licensed_version_ids='{}';
 UPDATE control_mapping SET to_version_id=NULL,to_version='3.0',to_citation='B.9',resolved=false;
 INSERT INTO quality_gap(kind,class,version_id,subject,detail,blocks) VALUES('mapping_unresolved','public',1,'fw:fw-beta:3.0#B.9','0',false);
 INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status)
 VALUES(3,1,2,'A . 2','A.2','control','Our title','active');`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSchema_EvidenceRequiresEveryDependency(t *testing.T) {
	conn := schemaDB(t)
	seedUnsealed(t, conn)
	ctx := context.Background()
	//nolint:misspell // Match the registry field names.
	_, err := conn.Exec(ctx, `INSERT INTO framework_version(id,framework_code,version,language,access,licence_kind,released,ordinal)
 VALUES(3,'fw-alpha','3.0','en','licensed','synthetic','2099-01-02',2);
 UPDATE release SET version_count=3,versions_sealed=true;
 INSERT INTO mapping_evidence VALUES(1,'Invented evidence','{1,3}');`)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		access []int64
		want   int
	}{
		{[]int64{}, 0}, {[]int64{1}, 0}, {[]int64{3}, 0}, {[]int64{1, 3}, 1},
	} {
		var count int
		if err := conn.QueryRow(ctx, `SELECT count(*) FROM mapping_evidence WHERE licensed_version_ids <@ $1::bigint[]`, tc.access).Scan(&count); err != nil || count != tc.want {
			t.Fatalf("access %v: count=%d, error=%v", tc.access, count, err)
		}
	}
	if _, err := conn.Exec(ctx, `UPDATE mapping_evidence SET licensed_version_ids='{}'`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM mapping_evidence WHERE licensed_version_ids <@ '{}'::bigint[]`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("public evidence: count=%d, error=%v", count, err)
	}
}
