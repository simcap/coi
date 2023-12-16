package main

import (
	"flag"
	"log"
	"os"

	"golang.org/x/mod/modfile"

	"github.com/simcap/coi"
)

var (
	printPositionsFlag     bool
	htmlFormatFlag         bool
	analyserFlag           string
	packagesFlag           string
	packagesAnalyserValues []string
)

func main() {
	log.SetFlags(0)
	flag.BoolVar(&printPositionsFlag, "pos", false, "Print filename position for each value")
	flag.BoolVar(&htmlFormatFlag, "html", false, "Generate HTML report file coi.html")
	flag.StringVar(&analyserFlag, "a", "", "Which analyser to run")
	flag.StringVar(&packagesFlag, "p", "./...", "Which packages ro tun on")
	flag.Parse()

	dir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	f, err := os.ReadFile("go.mod")
	if err != nil {
		log.Println(err)
	}
	config := coi.Config{Module: modfile.ModulePath(f), WorkingDir: dir}

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

	go func() {
		runner.Run([]string{packagesFlag})
	}()
	report := coi.BuildReport(runner)

	if htmlFormatFlag {
		f, err := os.Create("coi.html")
		if err != nil {
			log.Fatal(err)
		}
		report.ToHTML(f)
	} else {
		report.ToText(os.Stdout)
	}
}
