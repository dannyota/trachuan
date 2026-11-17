package build

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

func TestSummary_GapSourceEnumeration(t *testing.T) {
	file, err := parser.ParseFile(token.NewFileSet(), "synthetic.go", `package synthetic
const namedGap = "constant_gap"
func reason() (int,string) { return 0,"returned_gap" }
func record() {
 gaps = append(gaps, Gap{Kind:"literal_gap"}, Gap{Kind:namedGap})
 gaps = []Gap{{Kind:"nested_gap"}}
 selected = selection{gap:"selected_gap"}
 _,gap := reason()
 selected.extraGap = "extra_gap"
 l.addGap("build",key,"build_gap","")
 report.addGap("report_gap","","")
 name := "initial_gap"
 if alternate { name = "alternate_gap" }
 report.addGap(name,"","")
 report.addGap(externalGap,"","")
 l.taxonomyGap(key,"taxonomy_gap","",1)
 stage.RetryableGap(err,"retry_gap")
 result.Gaps = append(result.Gaps,"parsed_gap")
 rejected = Rejected{1,nil,"embedding_gap"}
 value = Relation{Kind:"not_a_gap"}
}`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	external, err := parser.ParseFile(token.NewFileSet(), "external.go", `package synthetic; const externalGap = "external_gap"`, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	got := corpusGapSourceNames(map[string]*ast.File{"synthetic.go": file, "external.go": external})
	want := []string{"alternate_gap", "build_gap", "constant_gap", "embedding_gap", "external_gap", "extra_gap", "initial_gap", "literal_gap", "nested_gap", "parsed_gap", "report_gap", "retry_gap", "returned_gap", "selected_gap", "taxonomy_gap"}
	if !slices.Equal(got, want) {
		t.Fatalf("gap names=%v, want %v", got, want)
	}
}

func corpusGapSourceNames(files map[string]*ast.File) []string {
	source := indexGapSources(files)
	kinds := map[string]bool{}
	add := func(expr ast.Expr, index int) {
		for _, value := range gapSourceValues(expr, index, &source, map[ast.Node]bool{}) {
			if value != "" {
				kinds[value] = true
			}
		}
	}
	for _, file := range files {
		stack := []ast.Node{}
		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil {
				stack = stack[:len(stack)-1]
				return false
			}
			stack = append(stack, node)
			switch n := node.(type) {
			case *ast.KeyValueExpr:
				key, ok := n.Key.(*ast.Ident)
				if !ok {
					break
				}
				name := key.Name
				kindType := ""
				for i := len(stack) - 2; i >= 0; i-- {
					if literal, ok := stack[i].(*ast.CompositeLit); ok && literal.Type != nil {
						kindType = gapSourceName(literal.Type)
						break
					}
				}
				if gapSourceField(n.Key) || name == "GapKind" || ((name == "Kind" || name == "Name") && (strings.HasSuffix(kindType, "Gap") || kindType == "Reason" || kindType == "InsertGapParams")) {
					add(n.Value, 0)
				}
			case *ast.CompositeLit:
				if gapSourceName(n.Type) == "Rejected" {
					for _, expr := range n.Elts {
						if _, keyed := expr.(*ast.KeyValueExpr); !keyed {
							add(expr, 0)
						}
					}
				}
			case *ast.AssignStmt:
				for i, left := range n.Lhs {
					if gapSourceField(left) {
						if len(n.Rhs) == 1 {
							add(n.Rhs[0], i)
						} else if i < len(n.Rhs) {
							add(n.Rhs[i], 0)
						}
					}
				}
			case *ast.CallExpr:
				index := -1
				switch gapSourceName(n.Fun) {
				case "addGap":
					if len(n.Args) == 4 {
						index = 2
					} else if len(n.Args) == 3 {
						index = 0
					}
				case "taxonomyGap", "RetryableGap":
					index = 1
				case "append", "appendOnce":
					if len(n.Args) > 0 && gapSourceName(n.Args[0]) == "Gaps" {
						for _, expr := range n.Args[1:] {
							add(expr, 0)
						}
					}
				}
				if index >= 0 && index < len(n.Args) {
					add(n.Args[index], 0)
				}
			}
			return true
		})
	}
	result := make([]string, 0, len(kinds))
	for kind := range kinds {
		result = append(result, kind)
	}
	slices.Sort(result)
	return result
}

func gapSourceField(expr ast.Expr) bool {
	name := strings.ToLower(gapSourceName(expr))
	return name == "gap" || name == "extragap"
}

func gapSourceName(expr ast.Expr) string {
	switch n := expr.(type) {
	case *ast.Ident:
		return n.Name
	case *ast.SelectorExpr:
		return n.Sel.Name
	case *ast.ArrayType:
		return gapSourceName(n.Elt)
	}
	return ""
}

type gapSourceValue struct {
	expr  ast.Expr
	index int
}

type gapSourceIndex struct {
	functions  map[string][]*ast.FuncDecl
	references map[*ast.Ident]*gapSourceBinding
	globals    map[string][]ast.Expr
}

type gapSourceBinding struct{ values []gapSourceValue }

type gapSourceScope struct {
	parent *gapSourceScope
	names  map[string]*gapSourceBinding
}

func (scope *gapSourceScope) lookup(name string) *gapSourceBinding {
	for current := scope; current != nil; current = current.parent {
		if binding := current.names[name]; binding != nil {
			return binding
		}
	}
	return nil
}

func indexGapSources(files map[string]*ast.File) gapSourceIndex {
	source := gapSourceIndex{functions: map[string][]*ast.FuncDecl{}, references: map[*ast.Ident]*gapSourceBinding{}, globals: map[string][]ast.Expr{}}
	packages := map[string]*gapSourceScope{}
	for path, file := range files {
		key := filepath.Dir(path)
		if packages[key] == nil {
			packages[key] = &gapSourceScope{names: map[string]*gapSourceBinding{}}
		}
		for _, decl := range file.Decls {
			switch n := decl.(type) {
			case *ast.FuncDecl:
				source.functions[n.Name.Name] = append(source.functions[n.Name.Name], n)
			case *ast.GenDecl:
				for _, spec := range n.Specs {
					if value, ok := spec.(*ast.ValueSpec); ok {
						for i, name := range value.Names {
							packages[key].names[name.Name] = &gapSourceBinding{}
							if i < len(value.Values) {
								source.globals[name.Name] = append(source.globals[name.Name], value.Values[i])
							}
						}
					}
				}
			}
		}
	}
	for path, file := range files {
		source.indexFile(file, packages[filepath.Dir(path)])
	}
	return source
}

type gapSourceFrame struct {
	previous *gapSourceScope
	pending  map[string]*gapSourceBinding
}

func (source *gapSourceIndex) indexFile(file *ast.File, scope *gapSourceScope) {
	frames := []gapSourceFrame{}
	ast.Inspect(file, func(node ast.Node) bool {
		if node == nil {
			frame := frames[len(frames)-1]
			for name, binding := range frame.pending {
				scope.names[name] = binding
			}
			scope = frame.previous
			frames = frames[:len(frames)-1]
			return false
		}
		frame := gapSourceFrame{previous: scope, pending: map[string]*gapSourceBinding{}}
		switch node.(type) {
		case *ast.FuncDecl, *ast.FuncLit, *ast.BlockStmt, *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.SwitchStmt, *ast.TypeSwitchStmt, *ast.CaseClause:
			scope = &gapSourceScope{parent: scope, names: map[string]*gapSourceBinding{}}
		}
		bindFields := func(fields *ast.FieldList) {
			if fields != nil {
				for _, field := range fields.List {
					for _, name := range field.Names {
						scope.names[name.Name] = &gapSourceBinding{}
					}
				}
			}
		}
		switch n := node.(type) {
		case *ast.FuncDecl:
			bindFields(n.Recv)
			bindFields(n.Type.Params)
			bindFields(n.Type.Results)
		case *ast.FuncLit:
			bindFields(n.Type.Params)
			bindFields(n.Type.Results)
		case *ast.RangeStmt:
			if n.Tok == token.DEFINE {
				for _, expr := range []ast.Expr{n.Key, n.Value} {
					if id, ok := expr.(*ast.Ident); ok {
						scope.names[id.Name] = &gapSourceBinding{}
					}
				}
			}
		case *ast.AssignStmt:
			for i, left := range n.Lhs {
				id, ok := left.(*ast.Ident)
				if !ok {
					continue
				}
				binding := scope.lookup(id.Name)
				if n.Tok == token.DEFINE && scope.names[id.Name] == nil {
					binding = &gapSourceBinding{}
					frame.pending[id.Name] = binding
				}
				if binding == nil {
					continue
				}
				source.references[id] = binding
				if len(n.Rhs) == 1 {
					binding.values = append(binding.values, gapSourceValue{n.Rhs[0], i})
				} else if i < len(n.Rhs) {
					binding.values = append(binding.values, gapSourceValue{n.Rhs[i], 0})
				}
			}
		case *ast.ValueSpec:
			for i, name := range n.Names {
				binding := scope.names[name.Name]
				if binding == nil {
					binding = &gapSourceBinding{}
					frame.pending[name.Name] = binding
				}
				if i < len(n.Values) {
					binding.values = append(binding.values, gapSourceValue{n.Values[i], 0})
				}
			}
		case *ast.Ident:
			if source.references[n] == nil {
				source.references[n] = scope.lookup(n.Name)
			}
		}
		frames = append(frames, frame)
		return true
	})
}

func gapSourceValues(expr ast.Expr, index int, source *gapSourceIndex, seen map[ast.Node]bool) []string {
	if expr == nil || seen[expr] {
		return nil
	}
	seen[expr] = true
	values := []string{}
	switch n := expr.(type) {
	case *ast.BasicLit:
		if n.Kind == token.STRING {
			if value, err := strconv.Unquote(n.Value); err == nil {
				values = append(values, value)
			}
		}
	case *ast.Ident:
		if binding := source.references[n]; binding != nil {
			for _, value := range binding.values {
				values = append(values, gapSourceValues(value.expr, value.index, source, seen)...)
			}
		}
	case *ast.SelectorExpr:
		for _, value := range source.globals[n.Sel.Name] {
			values = append(values, gapSourceValues(value, 0, source, seen)...)
		}
	case *ast.CallExpr:
		for _, fn := range source.functions[gapSourceName(n.Fun)] {
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				if statement, ok := node.(*ast.ReturnStmt); ok && index < len(statement.Results) {
					values = append(values, gapSourceValues(statement.Results[index], 0, source, seen)...)
				}
				return true
			})
		}
	}
	return values
}
