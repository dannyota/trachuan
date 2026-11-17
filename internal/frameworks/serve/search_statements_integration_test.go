//go:build integration

package serve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSearch_StatementCountDoesNotGrowWithRelease(t *testing.T) {
	e, trace := buildFixture(t, false)
	ctx := context.Background()
	search := func() {
		trace.reset()
		response, err := e.Search(ctx, Request{Query: "quartz", Limit: 50}, fakeCaller())
		require.NoError(t, err)
		require.NotEmpty(t, response.Hits)
		require.Len(t, trace.snapshot(), 7)
		trace.assertProjection(t, e.project(fakeCaller()).readable)
	}
	search()
	_, err := e.db.Exec(ctx, `INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status)
 SELECT n,3,n,'X-'||n,'X-'||n,'control','Invented quartz safeguard','' FROM generate_series(10000,10999) n;
 INSERT INTO control_text(control_id,title_original,body,notes,tables)
 SELECT n,'Invented title','Invented quartz safeguard.','{}','{}' FROM generate_series(10000,10999) n;
 INSERT INTO body_chunk(id,version_id,control_id,ordinal,content,input_sha256,token_count,embedding)
 SELECT n,3,n,0,'Invented quartz safeguard.',repeat('a',64),3,
 (SELECT embedding FROM body_chunk WHERE version_id=3 LIMIT 1) FROM generate_series(10000,10999) n;
 INSERT INTO public_chunk(id,version_id,control_id,content,input_sha256,token_count,embedding)
 SELECT n,3,n,'Invented quartz safeguard.',repeat('a',64),3,
 (SELECT embedding FROM public_chunk WHERE version_id=3 LIMIT 1) FROM generate_series(10000,10999) n;
 INSERT INTO body_posting(version_id,chunk_id,token,tf)
 SELECT 3,n,token,1 FROM generate_series(10000,10999) n CROSS JOIN unnest(ARRAY['invented','quartz','safeguard']) token;
 INSERT INTO public_posting(version_id,chunk_id,token,tf)
 SELECT 3,n,token,1 FROM generate_series(10000,10999) n CROSS JOIN unnest(ARRAY['invented','quartz','safeguard']) token`)
	require.NoError(t, err)
	e, err = Open(context.WithValue(ctx, openingTraceKey{}, true), e.db, syntheticQuery{}, Options{CursorKey: e.key})
	require.NoError(t, err)
	search()
}
