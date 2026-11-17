package retrieval

import "context"

func (e *Engine) TaxonomyCodes(ctx context.Context) ([]string, error) {
	codes := []string{}
	if e.release.SchemaVersion == 4 {
		return codes, nil
	}
	ctx, cancel := context.WithTimeout(ctx, e.deadline())
	defer cancel()
	tx, err := e.metadataTx(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := loadTaxonomy(ctx, tx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool, len(rows))
	for _, row := range rows {
		code := row.entry.Code
		if !validTaxonomyCode(code) || seen[code] {
			return nil, ErrUnavailable
		}
		seen[code] = true
		codes = append(codes, code)
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, databaseFailure("commit taxonomy codes", err)
	}
	return codes, nil
}
