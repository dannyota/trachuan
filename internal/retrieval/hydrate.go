package retrieval

import (
	"context"
	"errors"
	"math"
	"sort"

	"github.com/jackc/pgx/v5"
)

func hydrate(ctx context.Context, tx pgx.Tx, g *ArmGroup, vec string) error {
	ids := make([]int64, 0, len(g.Dense)+len(g.Lexical))
	seen := map[int64]bool{}
	for _, arm := range [][]Candidate{g.Dense, g.Lexical} {
		for _, c := range arm {
			if !seen[c.ChunkID] {
				seen[c.ChunkID] = true
				ids = append(ids, c.ChunkID)
			}
		}
	}
	if len(ids) == 0 {
		return nil
	}
	const sql = `WITH RECURSIVE ancestry AS (
 SELECT c.id AS chunk_id,p.id,p.parent_id,p.kind,0 AS depth FROM chunk c JOIN provision p ON p.id=c.provision_id WHERE c.id=ANY($1::bigint[])
 UNION ALL SELECT a.chunk_id,p.id,p.parent_id,p.kind,a.depth+1 FROM ancestry a JOIN provision p ON p.id=a.parent_id
 ), chosen AS (
 SELECT DISTINCT ON (chunk_id) chunk_id,id AS group_id FROM ancestry ORDER BY chunk_id,CASE WHEN kind='khoan' THEN 0 ELSE 1 END,depth
 ), groups AS (SELECT DISTINCT p.id,p.kind FROM chosen JOIN provision p ON p.id=chosen.group_id), mapped AS (
 SELECT id AS group_id,id,kind FROM groups
 UNION ALL SELECT m.group_id,p.id,p.kind FROM mapped m JOIN provision p ON p.parent_id=m.id AND p.kind<>'khoan' WHERE m.kind='khoan' OR m.group_id<>m.id
 )
 SELECT c.id,c.ordinal,d.country,d.key,p.anchor,p.id,m.group_id,gp.anchor,c.content,c.context_prefix,c.spans,1-(c.embedding <=> $2::vector),p.parent_id,p.kind,p.label,p.heading,p.text,p.node_id,d.number,d.title,d.status_class,d.is_current,d.eff_from,d.eff_to,d.source_url
 FROM mapped m JOIN chunk c ON c.provision_id=m.id JOIN document d ON d.id=c.document_id JOIN provision p ON p.id=c.provision_id JOIN provision gp ON gp.id=m.group_id WHERE m.id=m.group_id OR m.kind<>'khoan' ORDER BY c.id`
	rows, err := tx.Query(ctx, sql, ids, vec)
	if err != nil {
		return errors.New("hydrate candidates failed")
	}
	byID := map[int64]Candidate{}
	groupIDs := map[int64]bool{}
	for rows.Next() {
		var c Candidate
		if rows.Scan(&c.ChunkID, &c.Ordinal, &c.Identity.Country, &c.Identity.DocumentKey, &c.Identity.Anchor, &c.ProvisionID, &c.GroupID, &c.Group.Anchor, &c.Content, &c.ContextPrefix, &c.Spans, &c.Cosine, &c.Provision.ParentID, &c.Provision.Kind, &c.Provision.Label, &c.Provision.Heading, &c.Provision.Text, &c.Provision.NodeID, &c.Document.Number, &c.Document.Title, &c.Document.StatusClass, &c.Document.IsCurrent, &c.Document.EffectiveFrom, &c.Document.EffectiveTo, &c.Document.SourceURL) != nil {
			rows.Close()
			return errors.New("read hydrated candidate failed")
		}
		if math.IsNaN(c.Cosine) || math.IsInf(c.Cosine, 0) {
			rows.Close()
			return errors.New("invalid candidate cosine")
		}
		c.Cosine = max(-1, min(1, c.Cosine))
		c.Group.Country = c.Identity.Country
		c.Group.DocumentKey = c.Identity.DocumentKey
		c.Provision.ID = c.ProvisionID
		c.Provision.Identity = c.Identity
		byID[c.ChunkID] = c
		groupIDs[c.GroupID] = true
	}
	if rows.Err() != nil {
		rows.Close()
		return errors.New("read hydrated candidates failed")
	}
	rows.Close()
	gids := make([]int64, 0, len(groupIDs))
	for id := range groupIDs {
		gids = append(gids, id)
	}
	const ancestrySQL = `WITH RECURSIVE ancestry AS (
 SELECT p.id AS group_id,p.id,p.parent_id,p.kind,p.label,p.heading,p.text,p.anchor,p.node_id,d.country,d.key,0 AS depth FROM provision p JOIN document d ON d.id=p.document_id WHERE p.id=ANY($1::bigint[])
 UNION ALL SELECT a.group_id,p.id,p.parent_id,p.kind,p.label,p.heading,p.text,p.anchor,p.node_id,a.country,a.key,a.depth+1 FROM ancestry a JOIN provision p ON p.id=a.parent_id
 ) SELECT group_id,id,parent_id,kind,label,heading,text,anchor,node_id,country,key FROM ancestry ORDER BY group_id,depth DESC`
	rows, err = tx.Query(ctx, ancestrySQL, gids)
	if err != nil {
		return errors.New("hydrate ancestry failed")
	}
	ancestors := map[int64][]Provision{}
	for rows.Next() {
		var id int64
		var p Provision
		if rows.Scan(&id, &p.ID, &p.ParentID, &p.Kind, &p.Label, &p.Heading, &p.Text, &p.Identity.Anchor, &p.NodeID, &p.Identity.Country, &p.Identity.DocumentKey) != nil {
			rows.Close()
			return errors.New("read hydrated ancestry failed")
		}
		ancestors[id] = append(ancestors[id], p)
	}
	if rows.Err() != nil {
		rows.Close()
		return errors.New("read hydrated ancestry failed")
	}
	rows.Close()
	descendants, relations, err := hydrateTrees(ctx, tx, gids)
	if err != nil {
		return err
	}
	for id, c := range byID {
		c.Descendants = descendants[c.GroupID]
		c.Relations = relations[c.GroupID]
		c.Ancestors = ancestors[c.GroupID]
		byID[id] = c
	}
	ids = ids[:0]
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		g.Members = append(g.Members, byID[id])
	}
	for _, arm := range [][]Candidate{g.Dense, g.Lexical} {
		for i := range arm {
			c, ok := byID[arm[i].ChunkID]
			if !ok {
				return errors.New("missing hydrated candidate")
			}
			c.Rank = arm[i].Rank
			c.BM25 = arm[i].BM25
			arm[i] = c
		}
	}
	return nil
}
