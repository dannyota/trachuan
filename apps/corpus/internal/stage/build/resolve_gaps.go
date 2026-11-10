package build

import (
	"context"
	"encoding/json"
	"fmt"

	"danny.vn/trachuan/apps/corpus/internal/stage"
	"github.com/jackc/pgx/v5"
)

func buildGaps(ctx context.Context, cfg Config, name string) ([]stage.Gap, error) {
	c, err := pgx.ParseConfig(cfg.BuilderURL)
	if err != nil {
		return nil, fmt.Errorf("parse builder configuration: %w", err)
	}
	c.Database = name
	db, err := pgx.ConnectConfig(ctx, c)
	if err != nil {
		return nil, fmt.Errorf("connect corpus database: %w", err)
	}
	defer func() { _ = db.Close(context.Background()) }()
	rows, err := db.Query(ctx, `SELECT kind,item_key,detail FROM coverage_gap WHERE stage='build' ORDER BY item_key COLLATE "C",kind COLLATE "C",detail COLLATE "C"`)
	if err != nil {
		return nil, fmt.Errorf("read build gaps: %w", err)
	}
	defer rows.Close()
	var gaps []stage.Gap
	for rows.Next() {
		var kind, item, detail string
		if err = rows.Scan(&kind, &item, &detail); err != nil {
			return nil, fmt.Errorf("scan build gap: %w", err)
		}
		encoded, e := json.Marshal(struct {
			ItemKey string `json:"item_key"`
			Detail  string `json:"detail"`
		}{item, detail})
		if e != nil {
			return nil, fmt.Errorf("encode build gap: %w", e)
		}
		gaps = append(gaps, stage.Gap{Kind: kind, Detail: string(encoded)})
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read build gaps: %w", err)
	}
	return gaps, nil
}
