//go:build integration

// Package fixture creates and removes the end-to-end corpus.
package fixture

import (
	"context"
	"errors"
	"fmt"

	"danny.vn/trachuan/internal/corpusdb"
	"danny.vn/trachuan/internal/testpg"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Create builds a marked fixture corpus and returns its database name and run key.
func Create(ctx context.Context) (name, key string, resultErr error) { return create(ctx, false) }

// CreateRecipe3 builds a recipe-3 corpus with a partial merged article.
func CreateRecipe3(ctx context.Context) (name, key string, resultErr error) { return create(ctx, true) }

func create(ctx context.Context, recipe3 bool) (name, key string, resultErr error) {
	admin, base, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		return "", "", err
	}
	defer func() { _ = admin.Close(context.Background()) }()
	name, key, err = testpg.CreateCorpus(ctx, admin, "e2e fixture")
	if err != nil {
		return name, key, err
	}
	defer func() {
		if resultErr != nil {
			dropped, cleanupErr := testpg.DropMarkedCorpus(context.Background(), admin, name, key)
			if cleanupErr != nil {
				resultErr = errors.Join(resultErr, cleanupErr)
			} else if !dropped {
				resultErr = errors.Join(resultErr, errors.New("fixture cleanup refused"))
			}
		}
	}()
	base.Path = "/" + name
	db, err := pgxpool.New(ctx, base.String())
	if err != nil {
		return name, key, fmt.Errorf("create fixture pool: %w", err)
	}
	defer db.Close()
	for _, extension := range []string{"vector", "ltree"} {
		if _, err := db.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS "+extension); err != nil {
			return name, key, fmt.Errorf("create fixture extension: %w", err)
		}
	}
	if err := corpusdb.Apply(ctx, db, 1024); err != nil {
		return name, key, fmt.Errorf("apply fixture schema: %w", err)
	}
	if err := populateWithRecipe(ctx, db, key[3:], recipe3); err != nil {
		return name, key, fmt.Errorf("populate fixture: %w", err)
	}
	return name, key, nil
}

// Drop removes a fixture only when its ownership marker matches.
func Drop(ctx context.Context, name, key string) error {
	admin, _, err := testpg.OpenAdministrator(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close(context.Background()) }()
	dropped, err := testpg.DropMarkedCorpus(ctx, admin, name, key)
	if err != nil {
		return err
	}
	if !dropped {
		return errors.New("fixture ownership marker mismatch or database missing")
	}
	return nil
}
