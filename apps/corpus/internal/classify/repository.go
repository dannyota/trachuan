package classify

import (
	"context"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/store"
	"github.com/jackc/pgx/v5/pgxpool"
)

type PGRepository struct {
	Pool *pgxpool.Pool
	conn *pgxpool.Conn
}

func (p *PGRepository) Lock(ctx context.Context) (func(), error) {
	if p.conn != nil {
		return nil, fmt.Errorf("submission ledger already locked")
	}
	conn, err := p.Pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire submission ledger connection: %w", err)
	}
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(861785302)`); err != nil {
		conn.Release()
		return nil, fmt.Errorf("lock submission ledger: %w", err)
	}
	p.conn = conn
	return func() {
		p.conn = nil
		raw := conn.Hijack()
		_ = raw.Close(context.Background())
	}, nil
}

func (p *PGRepository) List(ctx context.Context) ([]Submission, error) {
	if p.conn == nil {
		return nil, fmt.Errorf("submission ledger lock required")
	}
	rows, err := store.New(p.conn).ListClassificationSubmissions(ctx)
	if err != nil {
		return nil, fmt.Errorf("read submission ledger: %w", err)
	}
	var result []Submission
	for _, v := range rows {
		s := Submission{ID: v.ID, RunID: v.RunID, Country: v.Country, Release: v.Rel, ScreenHash: v.ScreenHash, ItemsHash: v.ItemListHash, State: v.State, CreatedAt: v.CreatedAt, SubmittedAt: v.SubmittedAt, DeadlineAt: v.DeadlineAt, Budget: v.Budget, BudgetMicroUSD: v.BudgetMicroUsd, ReserveMicroUSD: v.ReservedMicroUsd, CostMicroUSD: v.CostMicroUsd, Settled: v.CostSettled}
		if v.ProviderJobID != nil {
			s.BatchID = *v.ProviderJobID
		}
		for _, entry := range []struct {
			raw []byte
			dst any
		}{{v.Recipe, &s.Recipe}, {v.Families, &s.Families}, {v.ItemList, &s.Items}, {v.Results, &s.Results}} {
			if err = json.Unmarshal(entry.raw, entry.dst); err != nil {
				return nil, fmt.Errorf("decode submission ledger: %w", err)
			}
		}
		if s.ItemsHash != itemsHash(s.Items) {
			return nil, fmt.Errorf("submission ordered item hash mismatch")
		}
		result = append(result, s)
	}
	return result, nil
}

func (p *PGRepository) Save(ctx context.Context, s Submission) error {
	if p.conn == nil {
		return fmt.Errorf("submission ledger lock required")
	}
	return saveSubmission(ctx, store.New(p.conn), s)
}

func (p *PGRepository) Restore(ctx context.Context, submissions []Submission) error {
	if p.conn == nil {
		return fmt.Errorf("submission ledger lock required")
	}
	tx, err := p.conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin submission reconstruction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	q := store.New(tx)
	for _, s := range submissions {
		if err = saveSubmission(ctx, q, s); err != nil {
			return err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit submission reconstruction: %w", err)
	}
	return nil
}

func saveSubmission(ctx context.Context, q *store.Queries, s Submission) error {
	var recipe, families, items, results []byte
	for _, entry := range []struct {
		src any
		dst *[]byte
	}{{s.Recipe, &recipe}, {s.Families, &families}, {s.Items, &items}, {s.Results, &results}} {
		b, err := json.Marshal(entry.src)
		if err != nil {
			return fmt.Errorf("encode submission ledger: %w", err)
		}
		*entry.dst = b
	}
	var batch *string
	if s.BatchID != "" {
		batch = &s.BatchID
	}
	err := q.SaveClassificationSubmission(ctx, store.SaveClassificationSubmissionParams{ID: s.ID, RunID: s.RunID, ScreenHash: s.ScreenHash, Recipe: recipe, Families: families, ItemList: items, ItemListHash: s.ItemsHash, ProviderJobID: batch, State: s.State, CreatedAt: s.CreatedAt, SubmittedAt: s.SubmittedAt, DeadlineAt: s.DeadlineAt, Budget: s.Budget, BudgetMicroUsd: s.BudgetMicroUSD, ReservedMicroUsd: s.ReserveMicroUSD, CostMicroUsd: s.CostMicroUSD, CostSettled: s.Settled, Results: results})
	if err != nil {
		return fmt.Errorf("save submission ledger: %w", err)
	}
	return nil
}
