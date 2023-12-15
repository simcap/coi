package coi

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

func NewMethodsAnalyser(c Config) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "methods",
		Doc:      "Collect methods of interest",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      methods(c),
	}
}

func NewPackageAnalyser(c Config) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "packages",
		Doc:      "Collect usage of given package",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      packages(c),
	}
}

func NewFunctionsAnalyser(c Config) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "functions",
		Doc:      "Collect functions of interest",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      functions(c),
	}
}

func NewStringAnalyser(Config) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "strings",
		Doc:      "Collect literal strings values",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      stringValues(),
	}
}

type Config struct {
	Methods   [][2]string
	Functions [][2]string
	Packages  []string
}

func stringValues() func(*analysis.Pass) (interface{}, error) {
	return func(pass *analysis.Pass) (interface{}, error) {
		inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

		inspect.WithStack(nil, func(n ast.Node, push bool, stack []ast.Node) bool {
			if lit, ok := n.(*ast.BasicLit); ok {
				parent := stack[len(stack)-2]
				if _, isImport := parent.(*ast.ImportSpec); !isImport {
					if lit.Kind == token.STRING {
						pass.Report(analysis.Diagnostic{
							Category: "strings",
							Pos:      lit.ValuePos,
							Message:  fmt.Sprintf("string: %s", lit.Value),
						})
						return false
					}
				}
			}
			return true
		})

		return nil, nil
	}
}

func methods(c Config) func(*analysis.Pass) (interface{}, error) {
	return func(pass *analysis.Pass) (interface{}, error) {
		inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

		inspect.WithStack(nil, func(n ast.Node, push bool, stack []ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				fun, _ := call.Fun.(*ast.SelectorExpr)
				if t := pass.TypesInfo.Types[fun.X].Type; t != nil {
					for _, m := range c.Methods {
						typ, method := m[0], m[1]
						if t.String() == typ {
							if fun.Sel.Name == method {
								pass.Report(analysis.Diagnostic{
									Pos:      call.Pos(),
									Category: "methods",
									Message:  fmt.Sprintf("%s.%s(%s)", typ, method, argsAsCommaSeparatedValues(call.Args)),
								})
								return false
							}
						}
					}
				}
			}
			return true
		})

		return nil, nil
	}
}

func functions(c Config) func(*analysis.Pass) (interface{}, error) {
	return func(pass *analysis.Pass) (interface{}, error) {
		inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

		inspect.WithStack(nil, func(n ast.Node, push bool, stack []ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				fun, _ := call.Fun.(*ast.SelectorExpr)
				for _, f := range c.Functions {
					pkg, name := f[0], f[1]
					if fun.Sel.Name == name {
						if p, isPackage := pass.TypesInfo.Uses[fun.X.(*ast.Ident)].(*types.PkgName); isPackage {
							if p.Imported().Path() == pkg {
								pass.Report(analysis.Diagnostic{
									Pos:      call.Pos(),
									Category: "functions",
									Message:  fmt.Sprintf("%s.%s(%s)", pkg, name, argsAsCommaSeparatedValues(call.Args)),
								})
								return false
							}
						}
					}
				}
			}
			return true
		})

		return nil, nil
	}
}

func packages(c Config) func(*analysis.Pass) (interface{}, error) {
	return func(pass *analysis.Pass) (interface{}, error) {
		inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

		inspect.WithStack(nil, func(n ast.Node, push bool, stack []ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				fun, _ := call.Fun.(*ast.SelectorExpr)
				for _, name := range c.Packages {
					if fun != nil && fun.X != nil {
						switch v := fun.X.(type) {
						case *ast.Ident:
							if p, isPackage := pass.TypesInfo.Uses[v].(*types.PkgName); isPackage {
								if p.Imported().Path() == name {
									pass.Report(analysis.Diagnostic{
										Pos:      call.Pos(),
										Category: "packages",
										Message:  fmt.Sprintf("%s.%s(%s)", name, fun.Sel.Name, argsAsCommaSeparatedValues(call.Args)),
									})
									return false
								}
							}
						}
					}
				}
			}
			return true
		})

		return nil, nil
	}
}

func argsAsCommaSeparatedValues(args []ast.Expr) string {
	var out []string
	for _, expr := range args {
		out = append(out, types.ExprString(expr))
	}
	return strings.Join(out, ", ")
}
