package main

import (
	"flag"
	"fmt"
	"log"
	"sync"

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

	config := coi.Config{PrintDiagnostics: printOnlyDiagnosticsFlag}

	var all []coi.AnalyserFunc
	switch analyserFlag {
	case "s":
		all = append(all, coi.FindStrings)
	case "p":
		config.Packages = flag.Args()
		all = append(all, coi.FindPackages)
	case "m":
		config.Methods = append(config.Methods, flag.Args()...)
		all = append(all, coi.FindMethods)
	case "f":
		config.Functions = append(config.Functions, flag.Args()...)
		all = append(all, coi.FindFunctions)
	}

	runner, err := coi.NewAnalysis(config, all...)
	if err != nil {
		log.Fatal(err)
	}

	var wg sync.WaitGroup
	if !runner.PrintDiagnostics {
		wg.Add(1)
		go func() {
			for item := range runner.ReportChan {
				if printPositionsFlag {
					fmt.Println(item.Position, item.Value)
				} else {
					fmt.Println(item.Value)
				}
			}
			wg.Done()
		}()
	}

	runner.Run([]string{packagesFlag})

	wg.Wait()
}
