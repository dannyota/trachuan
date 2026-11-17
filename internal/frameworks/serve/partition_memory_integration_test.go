//go:build integration

package serve

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

const partitionBoundaryFixtureSQL = `DELETE FROM body_posting; DELETE FROM public_posting;
 DELETE FROM body_chunk; DELETE FROM public_chunk; DELETE FROM control_text; DELETE FROM control;
 INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status)
 VALUES(1,3,1,'A','A','control','Invented safeguard','');
 INSERT INTO control_text VALUES(1,'Invented title','Invented body','{}','{}');
 INSERT INTO body_chunk(id,version_id,control_id,ordinal,content,input_sha256,token_count,embedding)
 SELECT n,3,1,n,'Invented body',repeat('a',64),1,array_prepend(1::real,array_fill(0::real,ARRAY[1023]))::vector
 FROM generate_series(1,10) n;
 INSERT INTO body_posting SELECT 3,n,CASE WHEN n%2=1 THEN 'alpha' ELSE 'beta' END,1 FROM generate_series(1,10) n;
 INSERT INTO public_chunk(id,version_id,control_id,content,input_sha256,token_count,embedding)
 SELECT 1,3,1,'Invented safeguard',repeat('a',64),1,embedding FROM body_chunk WHERE id=1;
 INSERT INTO public_posting VALUES(3,1,'alpha',1)`

func TestOpen_RejectsEachPartitionCategoryAcrossBound(t *testing.T) {
	e, _ := buildFixture(t, false)
	ctx := context.Background()
	reset := func(t *testing.T) {
		t.Helper()
		_, err := e.db.Exec(ctx, partitionBoundaryFixtureSQL)
		require.NoError(t, err)
	}
	reset(t)
	open := func(limit int64) (*Engine, error) {
		return openWithPartitionLimit(ctx, e.db, syntheticQuery{}, Options{CursorKey: e.key}, limit)
	}
	lo, hi := int64(1), int64(16384)
	for lo < hi {
		mid := lo + (hi-lo)/2
		loaded, err := open(mid)
		if err != nil {
			require.Nil(t, loaded)
			require.ErrorIs(t, err, ErrUnavailable)
			lo = mid + 1
		} else {
			require.NotNil(t, loaded)
			hi = mid
		}
	}
	base := lo
	for _, tc := range []struct {
		name  string
		delta int64
		sql   string
	}{
		{"control", 512, `INSERT INTO control(id,version_id,ordinal,citation,citation_norm,kind,title,status)
 VALUES(2,3,2,'B','B','control','Invented safeguard','')`},
		{"chunk", 245, `INSERT INTO body_chunk(id,version_id,control_id,ordinal,content,input_sha256,token_count,embedding)
 SELECT 11,3,1,11,'Invented body',repeat('a',64),1,embedding FROM body_chunk WHERE id=1;
 INSERT INTO body_posting VALUES(3,11,'alpha',1)`},
		{"posting", 116, `UPDATE body_chunk SET token_count=2 WHERE id=1;
 INSERT INTO body_posting VALUES(3,1,'beta',1)`},
		{"statistic", 101, `UPDATE body_posting SET token='gamma' WHERE chunk_id=1`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reset(t)
			limit := base + tc.delta - 1
			loaded, err := open(limit)
			require.NoError(t, err)
			require.NotNil(t, loaded)
			_, err = e.db.Exec(ctx, tc.sql)
			require.NoError(t, err)
			loaded, err = open(limit)
			require.ErrorIs(t, err, ErrUnavailable)
			require.Nil(t, loaded)
		})
	}
}
