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

type AnalyserFunc func(r *Runner) *analysis.Analyzer

func NewAnalysis(config Config, all ...AnalyserFunc) (*Runner, error) {
	r, err := NewRunner(config)
	if err != nil {
		return r, err
	}

	for _, f := range all {
		r.analysers = append(r.analysers, f(r))
	}

	if err := analysis.Validate(r.analysers); err != nil {
		return r, err
	}
	return r, nil
}

func FindMethods(r *Runner) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "methods",
		Doc:      "Collect methods of interest",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      methods(r),
	}
}

func FindPackages(r *Runner) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "packages",
		Doc:      "Collect usage of given package",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      packagesValues(r),
	}
}

func FindFunctions(r *Runner) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "functions",
		Doc:      "Collect functions of interest",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      functions(r),
	}
}

func FindStrings(r *Runner) *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "strings",
		Doc:      "Collect literal strings values",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      stringValues(r),
	}
}
func stringValues(r *Runner) func(*analysis.Pass) (interface{}, error) {
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
						r.ReportChan <- Item{Category: "strings", Value: lit.Value, Position: pass.Fset.Position(lit.Pos())}
						return false
					}
				}
			}
			return true
		})

		return nil, nil
	}
}

func methods(r *Runner) func(*analysis.Pass) (interface{}, error) {
	return func(pass *analysis.Pass) (interface{}, error) {
		inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

		inspect.WithStack(nil, func(n ast.Node, push bool, stack []ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				fun, _ := call.Fun.(*ast.SelectorExpr)
				if fun != nil {
					if t := pass.TypesInfo.Types[fun.X].Type; t != nil {
						for _, m := range r.methods {
							typ, method := m.left, m.right
							if t.String() == typ {
								if fun.Sel.Name == method {
									msg := fmt.Sprintf("%s.%s(%s)", typ, method, argsAsCommaSeparatedValues(call.Args))
									pass.Report(analysis.Diagnostic{
										Pos:      call.Pos(),
										Category: "methods",
										Message:  msg,
									})
									r.ReportChan <- Item{Category: "methods", Value: msg, Position: pass.Fset.Position(call.Pos())}
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

func functions(r *Runner) func(*analysis.Pass) (interface{}, error) {
	return func(pass *analysis.Pass) (interface{}, error) {
		inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

		inspect.WithStack(nil, func(n ast.Node, push bool, stack []ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				fun, _ := call.Fun.(*ast.SelectorExpr)
				for _, f := range r.functions {
					pkg, name := f.left, f.right
					if fun != nil && fun.Sel != nil && fun.Sel.Name == name {
						if p, isPackage := pass.TypesInfo.Uses[fun.X.(*ast.Ident)].(*types.PkgName); isPackage {
							if p.Imported().Path() == pkg {
								msg := fmt.Sprintf("%s.%s(%s)", pkg, name, argsAsCommaSeparatedValues(call.Args))
								pass.Report(analysis.Diagnostic{
									Pos:      call.Pos(),
									Category: "functions",
									Message:  msg,
								})
								r.ReportChan <- Item{Category: "functions", Value: msg, Position: pass.Fset.Position(call.Pos())}
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

func packagesValues(r *Runner) func(*analysis.Pass) (interface{}, error) {
	return func(pass *analysis.Pass) (interface{}, error) {
		inspect := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)

		inspect.WithStack(nil, func(n ast.Node, push bool, stack []ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok {
				fun, _ := call.Fun.(*ast.SelectorExpr)
				for _, name := range r.packages {
					if fun != nil && fun.X != nil {
						switch v := fun.X.(type) {
						case *ast.Ident:
							if p, isPackage := pass.TypesInfo.Uses[v].(*types.PkgName); isPackage {
								if p.Imported().Path() == name {
									msg := fmt.Sprintf("%s.%s(%s)", name, fun.Sel.Name, argsAsCommaSeparatedValues(call.Args))
									pass.Report(analysis.Diagnostic{
										Pos:      call.Pos(),
										Category: "packages",
										Message:  msg,
									})
									r.ReportChan <- Item{Category: "packages", Value: msg, Position: pass.Fset.Position(call.Pos())}
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
