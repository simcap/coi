package main

import (
	"flag"
	"log"

	"golang.org/x/tools/go/analysis"

	"github.com/simcap/coi"
)

func main() {
	log.SetFlags(0)

	config := coi.Config{Packages: []string{"os", "net/url", "io"}}
	analyserFlag := flag.String("a", "", "Analyser to run")
	flag.Parse()

	var analysers []*analysis.Analyzer
	switch *analyserFlag {
	case "s":
		analysers = append(analysers, coi.NewStringAnalyser(config))
	case "p":
		analysers = append(analysers, coi.NewPackageAnalyser(config))
	}

	if err := analysis.Validate(analysers); err != nil {
		log.Fatal(err)
	}

	Run([]string{"./..."}, analysers)
}
