package retrieval

import (
	"context"
	"errors"
	"sort"
	"strconv"

	"github.com/jackc/pgx/v5"
)

// HydrateSiblingMerge loads article evidence only for a research evaluation.
// This does not collect chunks, alter arm membership, or affect serving caches.
func (e *Engine) HydrateSiblingMerge(ctx context.Context, arms Arms) (*SiblingMerge, error) {
	if e.options.Profile != "evaluation" {
		return nil, errors.New("sibling merge requires evaluation profile")
	}
	out := &SiblingMerge{Articles: map[int64]Candidate{}}
	for _, arm := range []ArmGroup{arms.Primary, arms.NonCurrent} {
		for _, c := range arm.Members {
			article, ok := groupArticle(Group{Best: c})
			if !ok {
				continue
			}
			if _, exists := out.Articles[article.ID]; exists {
				continue
			}
			ancestors := []Provision{}
			for _, p := range c.Ancestors {
				ancestors = append(ancestors, p)
				if p.ID == article.ID {
					break
				}
			}
			out.Articles[article.ID] = Candidate{GroupID: article.ID, Group: article.Identity, Identity: article.Identity, Ancestors: ancestors, Document: c.Document}
		}
	}
	if len(out.Articles) == 0 {
		return out, nil
	}
	if e.db == nil {
		return nil, errors.New("missing evaluation database")
	}
	ids := make([]int64, 0, len(out.Articles))
	for id := range out.Articles {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.db.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, databaseFailure("begin sibling hydration failed", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err = setLocal(ctx, tx, "statement_timeout", strconv.FormatInt(e.deadline().Milliseconds(), 10)); err != nil {
		return nil, err
	}
	trees, relations, err := hydrateTrees(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	warnings, err := loadWarnings(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		c := out.Articles[id]
		c.Descendants = trees[id]
		c.Relations = relations[id]
		c.Warnings = warnings[id].warnings
		c.WarningsOmitted = warnings[id].omitted
		if len(c.Descendants) == 0 {
			return nil, ErrCacheInsufficient
		}
		out.Articles[id] = c
	}
	out.EffectiveNodes, out.Nodes, err = e.loadEffectiveNodes(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, databaseFailure("commit sibling hydration failed", err)
	}
	return out, nil
}
