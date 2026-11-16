package classify

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"danny.vn/trachuan/apps/corpus/internal/artifact"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/unicode/norm"
)

func ReadFields(dir, key, hash string) ([]string, error) {
	actual, err := artifact.VerifyFiles(dir, "detail.json")
	if err != nil {
		return nil, fmt.Errorf("verify classification fetch artifact: %w", err)
	}
	if actual != hash {
		return nil, fmt.Errorf("classification fetch artifact differs from release")
	}
	b, err := os.ReadFile(filepath.Join(dir, "detail.json"))
	if err != nil {
		return nil, fmt.Errorf("read classification fetch detail: %w", err)
	}
	var envelope struct {
		Data struct {
			ID     string          `json:"id"`
			Fields json.RawMessage `json:"documentFields"`
		} `json:"data"`
	}
	if err = json.Unmarshal(b, &envelope); err != nil {
		return nil, fmt.Errorf("decode classification fetch detail: %w", err)
	}
	if key != "vn:vbpl:"+envelope.Data.ID {
		return nil, fmt.Errorf("classification fetch identity mismatch")
	}
	if len(envelope.Data.Fields) == 0 {
		return nil, fmt.Errorf("VBPL documentFields unavailable; classification stopped")
	}
	var fields []struct {
		Name string `json:"name"`
	}
	if err = json.Unmarshal(envelope.Data.Fields, &fields); err != nil {
		return nil, fmt.Errorf("decode VBPL fields: %w", err)
	}
	names := make([]string, 0, len(fields))
	for _, field := range fields {
		if strings.TrimSpace(field.Name) == "" {
			return nil, fmt.Errorf("VBPL field has no name")
		}
		names = append(names, field.Name)
	}
	return names, nil
}

func LoadFamilies(ctx context.Context, db *pgxpool.Pool) ([]Family, error) {
	rows, err := db.Query(ctx, `SELECT n.code,n.name,p.name FROM taxonomy_node n JOIN taxonomy_node p ON p.id=n.parent_id WHERE n.level=2 AND n.status='active' AND p.level=1 AND p.status='active' ORDER BY p.ordinal,n.ordinal,n.code`)
	if err != nil {
		return nil, fmt.Errorf("read classification families: %w", err)
	}
	defer rows.Close()
	var result []Family
	for rows.Next() {
		var f Family
		if err = rows.Scan(&f.Code, &f.Name, &f.ParentName); err != nil {
			return nil, fmt.Errorf("decode classification family: %w", err)
		}
		result = append(result, f)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read classification families: %w", err)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("release has no active level-2 taxonomy nodes")
	}
	return result, nil
}

func LoadDocuments(ctx context.Context, release, workshop *pgxpool.Pool, artifacts artifact.Store, n int, seed int64) ([]Document, error) {
	rows, err := release.Query(ctx, `SELECT d.key,d.title,d.fetch_sha256 FROM document d WHERE d.class='primary' AND EXISTS (SELECT 1 FROM provision p WHERE p.document_id=d.id AND p.kind='dieu' AND NOT EXISTS (SELECT 1 FROM taxonomy_assignment a WHERE a.provision_id=p.id AND a.method='phapdien_seed')) ORDER BY d.key`)
	if err != nil {
		return nil, fmt.Errorf("read classification documents: %w", err)
	}
	var docs []Document
	hashes := map[string]string{}
	for rows.Next() {
		var d Document
		var hash string
		if err = rows.Scan(&d.Key, &d.Title, &hash); err != nil {
			rows.Close()
			return nil, fmt.Errorf("decode classification document: %w", err)
		}
		docs = append(docs, d)
		hashes[d.Key] = hash
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("read classification documents: %w", err)
	}
	selected, err := draw(docs, n, seed)
	if err != nil {
		return nil, err
	}
	for i := range selected {
		d := &selected[i]
		var path string
		if err = workshop.QueryRow(ctx, `SELECT path FROM artifact WHERE stage='fetch' AND sha256=$1 ORDER BY id LIMIT 1`, hashes[d.Key]).Scan(&path); err != nil {
			return nil, fmt.Errorf("locate pinned classification fetch artifact: %w", err)
		}
		if d.Fields, err = ReadFields(artifacts.Abs(path), d.Key, hashes[d.Key]); err != nil {
			return nil, err
		}
		if err = loadContents(ctx, release, d); err != nil {
			return nil, err
		}
	}
	return selected, nil
}

func loadContents(ctx context.Context, db *pgxpool.Pool, d *Document) error {
	rows, err := db.Query(ctx, `SELECT p.kind,p.label,p.heading,p.text,p.path::text FROM provision p JOIN document d ON d.id=p.document_id WHERE d.key=$1 ORDER BY p.preorder`, d.Key)
	if err != nil {
		return fmt.Errorf("read classification document contents: %w", err)
	}
	defer rows.Close()
	scopePath := ""
	for rows.Next() {
		var kind, label, heading, text, path string
		if err = rows.Scan(&kind, &label, &heading, &text, &path); err != nil {
			return fmt.Errorf("decode classification document contents: %w", err)
		}
		if kind != "khoan" && kind != "diem" {
			d.Contents = append(d.Contents, strings.TrimSpace(label+" "+heading))
		}
		if scopePath == "" && kind == "dieu" && strings.Contains(strings.ToLower(norm.NFC.String(heading)), "phạm vi") {
			scopePath = path
		}
		if scopePath != "" && (path == scopePath || strings.HasPrefix(path, scopePath+".")) {
			if d.Scope != "" {
				d.Scope += "\n"
			}
			d.Scope += text
		}
	}
	if err = rows.Err(); err != nil {
		return fmt.Errorf("read classification document contents: %w", err)
	}
	return nil
}
