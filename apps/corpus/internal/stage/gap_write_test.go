package stage

import (
	"context"
	"errors"

	"danny.vn/trachuan/apps/corpus/internal/detector"
	"danny.vn/trachuan/apps/corpus/internal/store"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

func TestInsertGap_OnlyTheGapWriterCallsIt(t *testing.T) {
	var callers []string
	err := filepath.WalkDir("../..", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		slash := filepath.ToSlash(path)
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.Contains(slash, "/internal/store/") {
			return nil
		}
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if literal, ok := node.(*ast.BasicLit); ok && literal.Kind == token.STRING && strings.Contains(strings.ToLower(literal.Value), "insert into gap") {
				callers = append(callers, slash+" (INSERT INTO gap)")
			}
			if call, ok := node.(*ast.CallExpr); ok {
				if selector, ok := call.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "InsertGap" && !isStagePackage(selector.X) {
					callers = append(callers, slash)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(callers) != 1 || !strings.HasSuffix(callers[0], "/internal/stage/gap_write.go") {
		t.Fatalf("InsertGap callers = %v, want only internal/stage/gap_write.go", callers)
	}
}

func TestInsertGap_WriterRefusesUnregisteredKind(t *testing.T) {
	err := InsertGap(context.Background(), nil, store.InsertGapParams{Kind: "future_gap"})
	if !errors.Is(err, detector.ErrUnregisteredGap) {
		t.Fatalf("err = %v", err)
	}
}

func isStagePackage(expr ast.Expr) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == "stage"
}
