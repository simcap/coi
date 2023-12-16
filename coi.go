package coi

import (
	"fmt"
	"go/ast"
	"go/token"
	"path/filepath"
	"strings"

	"golang.org/x/tools/go/analysis"
)

type Config struct {
	Module           string
	WorkingDir       string
	PrintDiagnostics bool
	Methods          []string `yaml:"methods"`
	Functions        []string `yaml:"functions"`
	Packages         []string `yaml:"packages"`
}

type Runner struct {
	ReportChan chan Item
	module     string
	workingDir string
	analysers  []*analysis.Analyzer
	methods    []Expr
	functions  []Expr
	packages   []string
}

type Item struct {
	Category         string
	Position         token.Position
	RelativeFilepath string
	GithubLink       string
	Value            string
}

type Expr struct {
	left  string
	right string
}

func NewRunner(c Config) (*Runner, error) {
	run := &Runner{
		ReportChan: make(chan Item),
		packages:   c.Packages,
		module:     c.Module,
		workingDir: c.WorkingDir,
	}
	for _, m := range c.Methods {
		if i := strings.LastIndex(m, "."); i > 0 {
			run.methods = append(run.methods, Expr{m[:i], m[i+1:]})
		} else {
			return run, fmt.Errorf("invalid method format: %s", m)
		}
	}
	for _, m := range c.Functions {
		if i := strings.LastIndex(m, "."); i > 0 {
			run.functions = append(run.functions, Expr{m[:i], m[i+1:]})
		} else {
			return run, fmt.Errorf("invalid function format: %s", m)
		}
	}
	return run, nil
}

func (r *Runner) GetRelativeFilepath(i Item) string {
	rel, _ := filepath.Rel(r.workingDir, i.Position.Filename)
	return rel
}

func (r *Runner) Close() { close(r.ReportChan) }

func NewStringItem(l *ast.BasicLit, set *token.FileSet) Item {
	return Item{Category: "strings", Value: l.Value, Position: set.Position(l.Pos())}
}
