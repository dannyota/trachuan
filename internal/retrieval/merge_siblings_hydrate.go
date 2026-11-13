package retrieval

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"
)

// hydrateSiblingArticles adds article evidence using Collect's read-only transaction.
func hydrateSiblingArticles(ctx context.Context, tx pgx.Tx, arms *Arms) ([]int64, error) {
	articles := map[int64]Candidate{}
	clauses := map[int64]map[int64]bool{}
	for _, arm := range []ArmGroup{arms.Primary, arms.NonCurrent} {
		for _, c := range arm.Members {
			article, ok := groupArticle(Group{Best: c})
			if !ok {
				continue
			}
			isClause := false
			for _, p := range c.Ancestors {
				if p.ID == c.GroupID && p.Kind == "khoan" {
					isClause = true
				}
			}
			if !isClause {
				continue
			}
			if clauses[article.ID] == nil {
				clauses[article.ID] = map[int64]bool{}
			}
			clauses[article.ID][c.GroupID] = true
			if _, exists := articles[article.ID]; exists {
				continue
			}
			ancestors := []Provision{}
			for _, p := range c.Ancestors {
				ancestors = append(ancestors, p)
				if p.ID == article.ID {
					break
				}
			}
			articles[article.ID] = Candidate{GroupID: article.ID, Group: article.Identity, Identity: article.Identity, Ancestors: ancestors, Document: c.Document}
		}
	}

	for id := range articles {
		if len(clauses[id]) < 2 {
			delete(articles, id)
		}
	}
	if len(articles) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(articles))
	for id := range articles {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	trees, relations, err := hydrateTrees(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	warnings, err := loadWarnings(ctx, tx, ids)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		c := articles[id]
		c.Descendants = trees[id]
		c.Relations = relations[id]
		c.Warnings = warnings[id].warnings
		c.WarningsOmitted = warnings[id].omitted
		if len(c.Descendants) == 0 {
			return nil, ErrCacheInsufficient
		}
		articles[id] = c
	}
	arms.Articles = articles
	return ids, nil
}
