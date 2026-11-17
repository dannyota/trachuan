package serve

import (
	"context"

	"danny.vn/trachuan/internal/retrieval/lexical"
)

type partition struct {
	handle    string
	controls  map[int64]*control
	citations map[string]*control
	counts    Counts
	body      indexPartition
	public    indexPartition
}

type indexPartition struct {
	chunks map[int64]indexChunk
	stats  lexical.Stats
}

type indexChunk struct {
	controlID       int64
	ordinal, length int
	postings        lexical.Vector
}

const mappingCountsSQL = `SELECT ends.code,ends.version,ends.citation,count(*) FILTER (WHERE ends.version_id IS NOT NULL),
 count(*) FILTER (WHERE ends.outgoing AND m.resolved),count(*) FILTER (WHERE ends.outgoing AND NOT m.resolved)
 FROM control_mapping m CROSS JOIN LATERAL (VALUES
 (m.from_code,m.from_version,m.from_citation,m.from_version_id,true),(m.to_code,m.to_version,m.to_citation,m.to_version_id,false))
 AS ends(code,version,citation,version_id,outgoing) GROUP BY ends.code,ends.version,ends.citation`

func (e *Engine) loadPartitions(ctx context.Context) error {
	e.partitions = make(map[int64]*partition, len(e.versions))
	for _, v := range e.versions {
		if err := e.reservePartitionMemory(int64(512 + len(v.handle))); err != nil {
			return err
		}
		e.partitions[v.id] = &partition{handle: v.handle, controls: map[int64]*control{}, citations: map[string]*control{}}
	}
	if err := e.loadControls(ctx); err != nil {
		return err
	}
	if err := e.loadMappingCounts(ctx); err != nil {
		return err
	}
	if err := e.loadIndex(ctx, false); err != nil {
		return err
	}
	return e.loadIndex(ctx, true)
}

func (e *Engine) loadMappingCounts(ctx context.Context) error {
	byHandle := map[string]*partition{}
	for _, part := range e.partitions {
		byHandle[part.handle] = part
	}
	rows, err := e.db.Query(ctx, mappingCountsSQL)
	if err != nil {
		return ErrUnavailable
	}
	defer rows.Close()
	for rows.Next() {
		var code, version, citation string
		var total, resolved, unresolved int
		if rows.Scan(&code, &version, &citation, &total, &resolved, &unresolved) != nil {
			return ErrUnavailable
		}
		part := byHandle["fw:"+code+":"+version]
		if part == nil {
			continue
		}
		if c := part.citations[citation]; c != nil {
			c.mappings = total
		}
		part.counts.EdgesResolved += resolved
		part.counts.EdgesUnresolved += unresolved
	}
	if rows.Err() != nil {
		return ErrUnavailable
	}
	return nil
}
