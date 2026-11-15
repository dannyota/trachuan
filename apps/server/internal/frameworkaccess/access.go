package frameworkaccess

import (
	"context"
	"errors"
	"sort"

	"danny.vn/trachuan/apps/server/internal/accountemail"
	"danny.vn/trachuan/apps/server/internal/store"
	"github.com/jackc/pgx/v5"
)

type Store interface {
	GetAccount(context.Context, int64) (store.Account, error)
	ListLiveFrameworkEntitlementsByEmail(context.Context, string) ([]store.FrameworkEntitlement, error)
}

type Service struct {
	Store      Store
	Frameworks []string
}

type Set struct {
	known, codes map[string]struct{}
	all          bool
}

type Decision string

const (
	Full    Decision = "full"
	Reduced Decision = "reduced"
)

type failure string

func (e failure) Error() string { return string(e) }

const ErrUnavailable failure = "framework access unavailable"

func Decide(code, class string, set Set) Decision {
	if _, ok := set.known[code]; !ok {
		return Reduced
	}
	if class == "free" {
		return Full
	}
	if class == "licensed" {
		if _, ok := set.codes[code]; set.all || ok {
			return Full
		}
	}
	return Reduced
}

func (s Set) Scopes() []string {
	if s.all {
		return []string{"*"}
	}
	scopes := make([]string, 0, len(s.codes))
	for code := range s.codes {
		scopes = append(scopes, code)
	}
	sort.Strings(scopes)
	return scopes
}

func (s *Service) AccessSet(ctx context.Context, accountID int64) (Set, error) {
	account, err := s.Store.GetAccount(ctx, accountID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Set{}, nil
	}
	if err != nil {
		return Set{}, ErrUnavailable
	}
	if account.BlockedAt != nil || account.DeleteRequestedAt != nil {
		return Set{}, nil
	}
	email, err := accountemail.Parse(account.Email)
	if err != nil {
		return Set{}, ErrUnavailable
	}
	rows, err := s.Store.ListLiveFrameworkEntitlementsByEmail(ctx, email)
	if err != nil {
		return Set{}, ErrUnavailable
	}
	set := Set{known: make(map[string]struct{}, len(s.Frameworks)), codes: make(map[string]struct{})}
	for _, code := range s.Frameworks {
		if _, err = store.ParseFrameworkScope(code); err == nil && code != "*" {
			set.known[code] = struct{}{}
		}
	}
	for _, row := range rows {
		if row.Email != email || row.RevokedAt != nil {
			continue
		}
		if row.Scope == "*" {
			set.all = true
			continue
		}
		if _, ok := set.known[row.Scope]; ok {
			set.codes[row.Scope] = struct{}{}
		}
	}
	return set, nil
}
