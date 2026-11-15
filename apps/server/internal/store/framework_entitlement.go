package store

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"danny.vn/trachuan/apps/server/internal/accountemail"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type frameworkFailure string

func (e frameworkFailure) Error() string { return string(e) }

const (
	ErrFrameworkAlreadyAllowed frameworkFailure = "framework entitlement already allowed"
	ErrFrameworkNotAllowed     frameworkFailure = "framework entitlement not live"
)

type FrameworkAllowance struct {
	Email, Scope, By, Note string
}

type FrameworkRevocation struct {
	Email, Scope, By string
}

func (q *Queries) AllowFrameworkEntitlement(ctx context.Context, p FrameworkAllowance) (FrameworkEntitlement, error) {
	email, err := frameworkEmail(p.Email, p.Scope, p.By)
	if err != nil {
		return FrameworkEntitlement{}, err
	}
	row, err := q.InsertFrameworkAllowance(ctx, InsertFrameworkAllowanceParams{Email: email, Scope: p.Scope, GrantedBy: p.By, Note: p.Note})
	if err != nil {
		var conflict *pgconn.PgError
		if errors.As(err, &conflict) && conflict.Code == "23505" && conflict.ConstraintName == "framework_entitlement_live_idx" {
			return FrameworkEntitlement{}, ErrFrameworkAlreadyAllowed
		}
		return FrameworkEntitlement{}, fmt.Errorf("allow framework entitlement: %w", err)
	}
	return FrameworkEntitlement(row), nil
}

func (q *Queries) RevokeFrameworkEntitlement(ctx context.Context, p FrameworkRevocation) error {
	email, err := frameworkEmail(p.Email, p.Scope, p.By)
	if err != nil {
		return err
	}
	_, err = q.ApplyFrameworkRevocation(ctx, ApplyFrameworkRevocationParams{Actor: &p.By, Email: email, Scope: p.Scope})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrFrameworkNotAllowed
	}
	if err != nil {
		return fmt.Errorf("revoke framework entitlement: %w", err)
	}
	return nil
}

func frameworkEmail(raw, scope, by string) (string, error) {
	email, err := accountemail.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("framework entitlement: %w", err)
	}
	if _, err = ParseFrameworkScope(scope); err != nil {
		return "", err
	}
	if strings.TrimSpace(by) == "" {
		return "", errors.New("missing framework operator")
	}
	return email, nil
}
