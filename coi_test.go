package coi

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestCollect(t *testing.T) {
	data := analysistest.TestData()

	t.Run("literal strings", func(t *testing.T) {
		analyser := NewStringAnalyser(Config{})
		analysistest.Run(t, data, analyser, "s")
	})

	t.Run("methods", func(t *testing.T) {
		analyser := NewMethodsAnalyser(Config{
			Methods: [][2]string{
				{"net/http.Header", "Set"},
				{"net/http.Header", "Add"},
			},
		})
		analysistest.Run(t, data, analyser, "http")
	})

	t.Run("functions", func(t *testing.T) {
		analyser := NewFunctionsAnalyser(Config{
			Functions: [][2]string{{"os", "ReadFile"}},
		})
		analysistest.Run(t, data, analyser, "o")
	})

	t.Run("functions", func(t *testing.T) {
		analyser := NewPackageAnalyser(Config{
			Packages: []string{"path/filepath", "encoding/hex"},
		})
		analysistest.Run(t, data, analyser, "p")
	})
}
