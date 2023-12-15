package main

// While waiting for https://github.com/golang/go/issues/61324
// we imported main runner/checker here and trimmed down everything not needed

// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Package checker defines the implementation of the checker commands.
// The same code drives the multi-analysis driver, the single-analysis
// driver that is conventionally provided for convenience along with
// each analysis package, and the test driver.
import (
	"errors"
	"fmt"
	"go/token"
	"go/types"
	"log"
	"net/url"
	"os"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"
)

// IncludeTests indicates whether test files should be analyzed too.
var IncludeTests = false

// Run loads the packages specified by args using go/packages,
// then applies the specified analyzers to them.
// Analysis flags must already have been set.
// Analyzers must be valid according to [analysis.Validate].
// It provides most of the logic for the main functions of both the
// singlechecker and the multi-analysis commands.
// It returns the appropriate exit code.
func Run(args []string, analyzers []*analysis.Analyzer, print bool) int {
	initial, err := load(args, false)
	if err != nil {
		if _, ok := err.(typeParseError); !ok {
			// Fail when some of the errors are not
			// related to parsing nor typing.
			log.Fatal(err)
		}
	}

	// Run the analysis.
	roots := analyze(initial, analyzers)

	if print {
		return printDiagnostics(roots)
	}

	return 0
}

// typeParseError represents a package load error
// that is related to typing and parsing.
type typeParseError struct {
	error
}

// load loads the initial packages. If all loading issues are related to
// typing and parsing, the returned error is of type typeParseError.
func load(patterns []string, allSyntax bool) ([]*packages.Package, error) {
	mode := packages.LoadSyntax
	if allSyntax {
		mode = packages.LoadAllSyntax
	}
	mode |= packages.NeedModule
	conf := packages.Config{
		Mode:  mode,
		Tests: IncludeTests,
	}
	initial, err := packages.Load(&conf, patterns...)
	if err == nil {
		if len(initial) == 0 {
			err = fmt.Errorf("%s matched no packages", strings.Join(patterns, " "))
		} else {
			err = loadingError(initial)
		}
	}
	return initial, err
}

// loadingError checks for issues during the loading of initial
// packages. Returns nil if there are no issues. Returns error
// of type typeParseError if all errors, including those in
// dependencies, are related to typing or parsing. Otherwise,
// a plain error is returned with an appropriate message.
func loadingError(initial []*packages.Package) error {
	var err error
	if n := packages.PrintErrors(initial); n > 1 {
		err = fmt.Errorf("%d errors during loading", n)
	} else if n == 1 {
		err = errors.New("error during loading")
	} else {
		// no errors
		return nil
	}
	all := true
	packages.Visit(initial, nil, func(pkg *packages.Package) {
		for _, err := range pkg.Errors {
			typeOrParse := err.Kind == packages.TypeError || err.Kind == packages.ParseError
			all = all && typeOrParse
		}
	})
	if all {
		return typeParseError{err}
	}
	return err
}

func analyze(pkgs []*packages.Package, analyzers []*analysis.Analyzer) []*action {
	// Each graph node (action) is one unit of analysis.
	// Edges express package-to-package (vertical) dependencies,
	// and analysis-to-analysis (horizontal) dependencies.
	type key struct {
		*analysis.Analyzer
		*packages.Package
	}
	actions := make(map[key]*action)

	var mkAction func(a *analysis.Analyzer, pkg *packages.Package) *action
	mkAction = func(a *analysis.Analyzer, pkg *packages.Package) *action {
		k := key{a, pkg}
		act, ok := actions[k]
		if !ok {
			act = &action{a: a, pkg: pkg}

			// Add a dependency on each required analyzers.
			for _, req := range a.Requires {
				act.deps = append(act.deps, mkAction(req, pkg))
			}

			// An analysis that consumes/produces facts
			// must run on the package's dependencies too.
			if len(a.FactTypes) > 0 {
				paths := make([]string, 0, len(pkg.Imports))
				for path := range pkg.Imports {
					paths = append(paths, path)
				}
				sort.Strings(paths) // for determinism
				for _, path := range paths {
					dep := mkAction(a, pkg.Imports[path])
					act.deps = append(act.deps, dep)
				}
			}

			actions[k] = act
		}
		return act
	}

	// Build nodes for initial packages.
	var roots []*action
	for _, a := range analyzers {
		for _, pkg := range pkgs {
			root := mkAction(a, pkg)
			root.isroot = true
			roots = append(roots, root)
		}
	}

	// Execute the graph in parallel.
	execAll(roots)

	return roots
}

// printDiagnostics prints the diagnostics for the root packages in either
// plain text or JSON format. JSON format also includes errors for any
// dependencies.
//
// It returns the exitcode: in plain mode, 0 for success, 1 for analysis
// errors, and 3 for diagnostics. We avoid 2 since the flag package uses
// it. JSON mode always succeeds at printing errors and diagnostics in a
// structured form to stdout.
func printDiagnostics(roots []*action) (exitcode int) {
	// Print the output.
	//
	// Print diagnostics only for root packages,
	// but errors for all packages.
	printed := make(map[*action]bool)
	var print func(*action)
	var visitAll func(actions []*action)
	visitAll = func(actions []*action) {
		for _, act := range actions {
			if !printed[act] {
				printed[act] = true
				visitAll(act.deps)
				print(act)
			}
		}
	}

	// plain text output

	// De-duplicate diagnostics by position (not token.Pos) to
	// avoid double-reporting in source files that belong to
	// multiple packages, such as foo and foo.test.
	type key struct {
		pos token.Position
		end token.Position
		*analysis.Analyzer
		message string
	}
	seen := make(map[key]bool)

	print = func(act *action) {
		if act.err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", act.a.Name, act.err)
			exitcode = 1 // analysis failed, at least partially
			return
		}
		if act.isroot {
			for _, diag := range act.diagnostics {
				// We don't display a.Name/f.Category
				// as most users don't care.

				posn := act.pkg.Fset.Position(diag.Pos)
				end := act.pkg.Fset.Position(diag.End)
				k := key{posn, end, act.a, diag.Message}
				if seen[k] {
					continue // duplicate
				}
				seen[k] = true

				PrintPlain(act.pkg.Fset, diag)
			}
		}
	}
	visitAll(roots)

	if exitcode == 0 && len(seen) > 0 {
		exitcode = 3 // successfully produced diagnostics
	}

	return exitcode
}

// An action represents one unit of analysis work: the application of
// one analysis to one package. Actions form a DAG, both within a
// package (as different analyzers are applied, either in sequence or
// parallel), and across packages (as dependencies are analyzed).
type action struct {
	once         sync.Once
	a            *analysis.Analyzer
	pkg          *packages.Package
	pass         *analysis.Pass
	isroot       bool
	deps         []*action
	objectFacts  map[objectFactKey]analysis.Fact
	packageFacts map[packageFactKey]analysis.Fact
	result       interface{}
	diagnostics  []analysis.Diagnostic
	err          error
	duration     time.Duration
}

type objectFactKey struct {
	obj types.Object
	typ reflect.Type
}

type packageFactKey struct {
	pkg *types.Package
	typ reflect.Type
}

func (act *action) String() string {
	return fmt.Sprintf("%s@%s", act.a, act.pkg)
}

func execAll(actions []*action) {
	var wg sync.WaitGroup
	for _, act := range actions {
		wg.Add(1)
		work := func(act *action) {
			act.exec()
			wg.Done()
		}
		go work(act)
	}
	wg.Wait()
}

func (act *action) exec() { act.once.Do(act.execOnce) }

func (act *action) execOnce() {
	// Analyze dependencies.
	execAll(act.deps)

	// TODO(adonovan): uncomment this during profiling.
	// It won't build pre-go1.11 but conditional compilation
	// using build tags isn't warranted.
	//
	// ctx, task := trace.NewTask(context.Background(), "exec")
	// trace.Log(ctx, "pass", act.String())
	// defer task.End()

	// Report an error if any dependency failed.
	var failed []string
	for _, dep := range act.deps {
		if dep.err != nil {
			failed = append(failed, dep.String())
		}
	}
	if failed != nil {
		sort.Strings(failed)
		act.err = fmt.Errorf("failed prerequisites: %s", strings.Join(failed, ", "))
		return
	}

	// Plumb the output values of the dependencies
	// into the inputs of this action.  Also facts.
	inputs := make(map[*analysis.Analyzer]interface{})
	act.objectFacts = make(map[objectFactKey]analysis.Fact)
	act.packageFacts = make(map[packageFactKey]analysis.Fact)
	for _, dep := range act.deps {
		if dep.pkg == act.pkg {
			// Same package, different analysis (horizontal edge):
			// in-memory outputs of prerequisite analyzers
			// become inputs to this analysis pass.
			inputs[dep.a] = dep.result

		} else if dep.a == act.a { // (always true)
			// Same analysis, different package (vertical edge):
			// serialized facts produced by prerequisite analysis
			// become available to this analysis pass.
			inheritFacts(act, dep)
		}
	}

	// Run the analysis.
	pass := &analysis.Pass{
		Analyzer:     act.a,
		Fset:         act.pkg.Fset,
		Files:        act.pkg.Syntax,
		OtherFiles:   act.pkg.OtherFiles,
		IgnoredFiles: act.pkg.IgnoredFiles,
		Pkg:          act.pkg.Types,
		TypesInfo:    act.pkg.TypesInfo,
		TypesSizes:   act.pkg.TypesSizes,
		TypeErrors:   act.pkg.TypeErrors,

		ResultOf:          inputs,
		Report:            func(d analysis.Diagnostic) { act.diagnostics = append(act.diagnostics, d) },
		ImportObjectFact:  act.importObjectFact,
		ExportObjectFact:  act.exportObjectFact,
		ImportPackageFact: act.importPackageFact,
		ExportPackageFact: act.exportPackageFact,
		AllObjectFacts:    act.allObjectFacts,
		AllPackageFacts:   act.allPackageFacts,
	}
	act.pass = pass

	var err error
	if act.pkg.IllTyped && !pass.Analyzer.RunDespiteErrors {
		err = fmt.Errorf("analysis skipped due to errors in package")
	} else {
		act.result, err = pass.Analyzer.Run(pass)
		if err == nil {
			if got, want := reflect.TypeOf(act.result), pass.Analyzer.ResultType; got != want {
				err = fmt.Errorf(
					"internal error: on package %s, analyzer %s returned a result of type %v, but declared ResultType %v",
					pass.Pkg.Path(), pass.Analyzer, got, want)
			}
		}
	}
	if err == nil { // resolve diagnostic URLs
		for i := range act.diagnostics {
			if url, uerr := ResolveURL(act.a, act.diagnostics[i]); uerr == nil {
				act.diagnostics[i].URL = url
			} else {
				err = uerr // keep the last error
			}
		}
	}
	act.err = err

	// disallow calls after Run
	pass.ExportObjectFact = nil
	pass.ExportPackageFact = nil
}

// inheritFacts populates act.facts with
// those it obtains from its dependency, dep.
func inheritFacts(act, dep *action) {
	for key, fact := range dep.objectFacts {
		// Filter out facts related to objects
		// that are irrelevant downstream
		// (equivalently: not in the compiler export data).
		if !exportedFrom(key.obj, dep.pkg.Types) {
			if false {
				log.Printf("%v: discarding %T fact from %s for %s: %s", act, fact, dep, key.obj, fact)
			}
			continue
		}
		act.objectFacts[key] = fact
	}

	for key, fact := range dep.packageFacts {
		act.packageFacts[key] = fact
	}
}

// exportedFrom reports whether obj may be visible to a package that imports pkg.
// This includes not just the exported members of pkg, but also unexported
// constants, types, fields, and methods, perhaps belonging to other packages,
// that find there way into the API.
// This is an overapproximation of the more accurate approach used by
// gc export data, which walks the type graph, but it's much simpler.
//
// TODO(adonovan): do more accurate filtering by walking the type graph.
func exportedFrom(obj types.Object, pkg *types.Package) bool {
	switch obj := obj.(type) {
	case *types.Func:
		return obj.Exported() && obj.Pkg() == pkg ||
			obj.Type().(*types.Signature).Recv() != nil
	case *types.Var:
		if obj.IsField() {
			return true
		}
		// we can't filter more aggressively than this because we need
		// to consider function parameters exported, but have no way
		// of telling apart function parameters from local variables.
		return obj.Pkg() == pkg
	case *types.TypeName, *types.Const:
		return true
	}
	return false // Nil, Builtin, Label, or PkgName
}

// importObjectFact implements Pass.ImportObjectFact.
// Given a non-nil pointer ptr of type *T, where *T satisfies Fact,
// importObjectFact copies the fact value to *ptr.
func (act *action) importObjectFact(obj types.Object, ptr analysis.Fact) bool {
	if obj == nil {
		panic("nil object")
	}
	key := objectFactKey{obj, factType(ptr)}
	if v, ok := act.objectFacts[key]; ok {
		reflect.ValueOf(ptr).Elem().Set(reflect.ValueOf(v).Elem())
		return true
	}
	return false
}

// exportObjectFact implements Pass.ExportObjectFact.
func (act *action) exportObjectFact(obj types.Object, fact analysis.Fact) {
	if act.pass.ExportObjectFact == nil {
		log.Panicf("%s: Pass.ExportObjectFact(%s, %T) called after Run", act, obj, fact)
	}

	if obj.Pkg() != act.pkg.Types {
		log.Panicf("internal error: in analysis %s of package %s: Fact.Set(%s, %T): can't set facts on objects belonging another package",
			act.a, act.pkg, obj, fact)
	}

	key := objectFactKey{obj, factType(fact)}
	act.objectFacts[key] = fact // clobber any existing entry
}

// allObjectFacts implements Pass.AllObjectFacts.
func (act *action) allObjectFacts() []analysis.ObjectFact {
	facts := make([]analysis.ObjectFact, 0, len(act.objectFacts))
	for k := range act.objectFacts {
		facts = append(facts, analysis.ObjectFact{Object: k.obj, Fact: act.objectFacts[k]})
	}
	return facts
}

// importPackageFact implements Pass.ImportPackageFact.
// Given a non-nil pointer ptr of type *T, where *T satisfies Fact,
// fact copies the fact value to *ptr.
func (act *action) importPackageFact(pkg *types.Package, ptr analysis.Fact) bool {
	if pkg == nil {
		panic("nil package")
	}
	key := packageFactKey{pkg, factType(ptr)}
	if v, ok := act.packageFacts[key]; ok {
		reflect.ValueOf(ptr).Elem().Set(reflect.ValueOf(v).Elem())
		return true
	}
	return false
}

// exportPackageFact implements Pass.ExportPackageFact.
func (act *action) exportPackageFact(fact analysis.Fact) {
	if act.pass.ExportPackageFact == nil {
		log.Panicf("%s: Pass.ExportPackageFact(%T) called after Run", act, fact)
	}

	key := packageFactKey{act.pass.Pkg, factType(fact)}
	act.packageFacts[key] = fact // clobber any existing entry
}

func factType(fact analysis.Fact) reflect.Type {
	t := reflect.TypeOf(fact)
	if t.Kind() != reflect.Ptr {
		log.Fatalf("invalid Fact type: got %T, want pointer", fact)
	}
	return t
}

// allPackageFacts implements Pass.AllPackageFacts.
func (act *action) allPackageFacts() []analysis.PackageFact {
	facts := make([]analysis.PackageFact, 0, len(act.packageFacts))
	for k := range act.packageFacts {
		facts = append(facts, analysis.PackageFact{Package: k.pkg, Fact: act.packageFacts[k]})
	}
	return facts
}

// ResolveURL resolves the URL field for a Diagnostic from an Analyzer
// and returns the URL. See Diagnostic.URL for details.
func ResolveURL(a *analysis.Analyzer, d analysis.Diagnostic) (string, error) {
	if d.URL == "" && d.Category == "" && a.URL == "" {
		return "", nil // do nothing
	}
	raw := d.URL
	if d.URL == "" && d.Category != "" {
		raw = "#" + d.Category
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid Diagnostic.URL %q: %s", raw, err)
	}
	base, err := url.Parse(a.URL)
	if err != nil {
		return "", fmt.Errorf("invalid Analyzer.URL %q: %s", a.URL, err)
	}
	return base.ResolveReference(u).String(), nil
}

const Context = -1

// PrintPlain prints a diagnostic in plain text form,
// with context specified by the -c flag.
func PrintPlain(fset *token.FileSet, diag analysis.Diagnostic) {
	posn := fset.Position(diag.Pos)
	fmt.Fprintf(os.Stderr, "%s: %s\n", posn, diag.Message)

	// -c=N: show offending line plus N lines of context.
	if Context >= 0 {
		posn := fset.Position(diag.Pos)
		end := fset.Position(diag.End)
		if !end.IsValid() {
			end = posn
		}
		data, _ := os.ReadFile(posn.Filename)
		lines := strings.Split(string(data), "\n")
		for i := posn.Line - Context; i <= end.Line+Context; i++ {
			if 1 <= i && i <= len(lines) {
				fmt.Fprintf(os.Stderr, "%d\t%s\n", i, lines[i-1])
			}
		}
	}
}
