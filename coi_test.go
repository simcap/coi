package coi

import (
	"testing"

	"golang.org/x/tools/go/analysis/analysistest"
)

func TestCollect(t *testing.T) {
	data := analysistest.TestData()

	t.Run("literal strings", func(t *testing.T) {
		analyser := FindStrings(mustNewRun(t, Config{}))
		analysistest.Run(t, data, analyser, "s")
	})

	t.Run("methods", func(t *testing.T) {
		config := Config{Methods: []string{"net/http.Header.Set", "net/http.Header.Add"}}
		analyser := FindMethods(mustNewRun(t, config))
		analysistest.Run(t, data, analyser, "http")
	})

	t.Run("functions", func(t *testing.T) {
		config := Config{Functions: []string{"os.ReadFile"}}
		analyser := FindFunctions(mustNewRun(t, config))
		analysistest.Run(t, data, analyser, "o")
	})

	t.Run("packages", func(t *testing.T) {
		config := Config{Packages: []string{"path/filepath", "encoding/hex"}}
		analyser := FindPackages(mustNewRun(t, config))
		analysistest.Run(t, data, analyser, "p")
	})
}

func mustNewRun(t *testing.T, c Config) *Runner {
	t.Helper()

	r, err := NewRunner(c)
	if err != nil {
		t.Fatal(err)
	}
	r.ReportChan = make(chan Item, 0)
	go func() {
		for range r.ReportChan {
		}
	}()
	t.Cleanup(func() { r.Close() })
	return r
}
