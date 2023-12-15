package main

import (
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"

	"golang.org/x/tools/go/analysis"

	"github.com/simcap/coi"
)

var (
	printOnlyDiagnosticsFlag bool
	printPositionsFlag       bool
	analyserFlag             string
	packagesFlag             string
	packagesAnalyserValues   []string
)

func main() {
	log.SetFlags(0)
	flag.BoolVar(&printOnlyDiagnosticsFlag, "d", false, "Only print regular go analyser results as diagnostics")
	flag.BoolVar(&printPositionsFlag, "pos", false, "Print filename position for each value")
	flag.StringVar(&analyserFlag, "a", "", "Which analyser to run")
	flag.StringVar(&packagesFlag, "p", "./...", "Which packages ro tun on")
	flag.Parse()

	config := coi.Config{ReportChan: make(chan coi.Item)}

	var analysers []*analysis.Analyzer
	switch analyserFlag {
	case "s":
		analysers = append(analysers, coi.NewStringAnalyser(config))
	case "p":
		config.Packages = flag.Args()
		analysers = append(analysers, coi.NewPackageAnalyser(config))
	case "m":
		for _, a := range flag.Args() {
			i := strings.LastIndex(a, ".")
			if i == 2 {
				config.Methods = append(config.Methods, [2]string{a[:i], a[i+1:]})
			}
		}
		analysers = append(analysers, coi.NewMethodsAnalyser(config))
	case "f":
		for _, a := range flag.Args() {
			i := strings.LastIndex(a, ".")
			if i == 2 {
				config.Functions = append(config.Functions, [2]string{a[:i], a[i+1:]})
			}
		}
		analysers = append(analysers, coi.NewFunctionsAnalyser(config))
	}

	if err := analysis.Validate(analysers); err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	if !printOnlyDiagnosticsFlag {
		wg.Add(1)
		go func() {
			for item := range config.ReportChan {
				if printPositionsFlag {
					fmt.Println(item.Position, item.Value)
				} else {
					fmt.Println(item.Value)
				}
			}
			wg.Done()
		}()
	}

	Run([]string{packagesFlag}, analysers, printOnlyDiagnosticsFlag)
	close(config.ReportChan)
	wg.Wait()
}
