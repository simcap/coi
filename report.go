package coi

import (
	"embed"
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"sort"
	"text/tabwriter"
)

//go:embed html
var htmlDir embed.FS

type Report struct {
	Strings   []Item
	Methods   []Item
	Functions []Item
	Packages  []Item
}

func BuildReport(r *Runner) *Report {
	report := new(Report)
	for item := range r.ReportChan {
		item.RelativeFilepath = r.GetRelativeFilepath(item)
		item.GithubLink = fmt.Sprintf("https://%s/blob/main/%s#L%d", r.module, item.RelativeFilepath, item.Position.Line)
		switch item.Category {
		case "strings":
			report.Strings = append(report.Strings, item)
		case "methods":
			report.Methods = append(report.Methods, item)
		case "functions":
			report.Functions = append(report.Functions, item)
		case "packages":
			report.Packages = append(report.Packages, item)
		}
	}
	sorting(report)
	return report
}

func (r *Report) ToText(w io.WriteCloser) {
	tw := tabwriter.NewWriter(w, 0, 0, 1, ' ', 0)
	printItems := func(items []Item) {
		for _, i := range items {
			fmt.Fprintf(tw, "%s\t%s\n", i.Position, i.Value)
		}
	}
	printItems(r.Strings)
	printItems(r.Functions)
	printItems(r.Methods)
	printItems(r.Packages)
	tw.Flush()
}

func (r *Report) ToHTML(w io.WriteCloser) {
	tmpl, err := template.New("report.html").ParseFS(htmlDir, "html/*")
	if err != nil {
		panic(err)
	}
	if err := tmpl.Execute(w, r); err != nil {
		panic(err)
	}
	w.Close()
}

func sorting(r *Report) {
	sort.Slice(r.Strings, func(i, j int) bool {
		return r.Strings[i].Value < r.Strings[j].Value
	})
}
