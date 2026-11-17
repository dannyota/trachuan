package queryfile

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jackc/pgx/v5"
)

func distinct(values []string) []string {
	seen := map[string]bool{}
	var result []string
	for _, v := range values {
		v = strings.Join(strings.Fields(v), " ")
		if v != "" && !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}
func Command(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("queries", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var db, suite, out string
	fs.StringVar(&db, "corpus-db", "", "corpus URL")
	fs.StringVar(&suite, "suite", "", "suite directory")
	fs.StringVar(&out, "out", "", "output file")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 || db == "" || suite == "" || out == "" {
		return errors.New("invalid queries options")
	}
	values, err := suiteQuestions(suite)
	if err != nil {
		return err
	}

	conn, err := pgx.Connect(ctx, db)
	if err != nil {
		return errors.New("connect corpus failed")
	}
	defer func() { _ = conn.Close(context.Background()) }()
	tx, err := conn.BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return errors.New("open read-only corpus transaction failed")
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := tx.Query(ctx, "SELECT title FROM document WHERE title <> '' ORDER BY id")
	if err != nil {
		return errors.New("read document titles failed")
	}
	for rows.Next() {
		var value string
		if rows.Scan(&value) != nil {
			rows.Close()
			return errors.New("read document titles failed")
		}
		values = append(values, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return errors.New("read document titles failed")
	}
	rows, err = tx.Query(ctx, "SELECT heading FROM provision WHERE kind = 'dieu' AND heading <> '' ORDER BY document_id, preorder")
	if err != nil {
		return errors.New("read article headings failed")
	}
	for rows.Next() {
		var value string
		if rows.Scan(&value) != nil {
			rows.Close()
			return errors.New("read article headings failed")
		}
		values = append(values, value)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return errors.New("read article headings failed")
	}
	if err = tx.Commit(ctx); err != nil {
		return errors.New("finish corpus read failed")
	}
	file, err := os.OpenFile(out, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create query file: %w", err)
	}
	writer := bufio.NewWriter(file)
	for _, v := range distinct(values) {
		if _, err = fmt.Fprintln(writer, v); err != nil {
			_ = file.Close()
			return errors.New("write query file failed")
		}
	}
	if err = writer.Flush(); err != nil {
		_ = file.Close()
		return errors.New("write query file failed")
	}
	if err = file.Close(); err != nil {
		return errors.New("close query file failed")
	}
	return nil
}

func suiteQuestions(dir string) ([]string, error) {
	var result []string
	for _, name := range []string{"legacy.json", "development.json"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return nil, errors.New("read suite failed")
		}
		var cases []struct {
			Question string `json:"question"`
		}
		if err = json.Unmarshal(data, &cases); err != nil || cases == nil {
			return nil, errors.New("invalid suite questions")
		}
		for _, c := range cases {
			if c.Question == "" {
				return nil, errors.New("empty suite question")
			}
			result = append(result, c.Question)
		}
	}
	if len(result) == 0 {
		return nil, errors.New("suite has no questions")
	}
	return result, nil
}
