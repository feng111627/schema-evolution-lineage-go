// Command sel is a local, offline schema-evolution and column-lineage checker.
//
// Usage:
//
//	sel parse    --file <path> [--format json|xml]
//	sel compat   --old <path> --new <path> [--format json|xml]
//	sel lineage  --file <catalog path> [--format json|xml]
//
// All files are read from the local filesystem; no network is used. Exit code
// is 0 on success and 1 when input is rejected, a breaking change is found, or
// a lineage issue is detected, making the conclusions reproducible from a
// shell or CI.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/feng111627/schema-evolution-lineage-go/internal/compat"
	"github.com/feng111627/schema-evolution-lineage-go/internal/lineage"
	"github.com/feng111627/schema-evolution-lineage-go/internal/parse"
	"github.com/feng111627/schema-evolution-lineage-go/internal/render"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]

	var code int
	switch cmd {
	case "parse":
		code = runParse(args)
	case "compat":
		code = runCompat(args)
	case "lineage":
		code = runLineage(args)
	case "-h", "--help", "help":
		usage()
		code = 0
	default:
		fmt.Fprintf(os.Stderr, "unknown subcommand %q\n\n", cmd)
		usage()
		code = 2
	}
	os.Exit(code)
}

func usage() {
	fmt.Fprintln(os.Stderr, `sel - local schema evolution & column-lineage checker

usage:
  sel parse    --file <path> [--format json|xml]
  sel compat   --old <path> --new <path> [--format json|xml]
  sel lineage  --file <catalog path> [--format json|xml]

exit codes: 0 ok, 1 rejection/breaking-change/lineage-issue, 2 usage error`)
}

func addFormatFlag(fs *flag.FlagSet) *string {
	return fs.String("format", "", "input format: json or xml (default: auto-detect)")
}

func parseFormat(s string) (parse.Format, error) {
	switch parse.Format(s) {
	case parse.FormatAuto:
		return parse.FormatAuto, nil
	case parse.FormatJSON:
		return parse.FormatJSON, nil
	case parse.FormatXML:
		return parse.FormatXML, nil
	default:
		return "", fmt.Errorf("--format must be json or xml, got %q", s)
	}
}

func readFile(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %v", path, err)
	}
	return data, nil
}

func runParse(args []string) int {
	fs := flag.NewFlagSet("parse", flag.ContinueOnError)
	file := fs.String("file", "", "path to a schema or catalog file")
	fmts := addFormatFlag(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *file == "" {
		fmt.Fprintln(os.Stderr, "parse: --file is required")
		return 2
	}
	f, err := parseFormat(*fmts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		return 2
	}
	data, err := readFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "parse:", err)
		return 1
	}
	cat, perr := parse.ParseCatalog(data, parse.Options{Format: f, Origin: *file})
	if perr != nil {
		fmt.Fprintf(os.Stderr, "parse: document rejected (%s):\n", *file)
		render.ParseErrors(os.Stderr, perr)
		return 1
	}
	render.Catalog(os.Stdout, cat)
	return 0
}

func runCompat(args []string) int {
	fs := flag.NewFlagSet("compat", flag.ContinueOnError)
	oldP := fs.String("old", "", "path to the old schema version")
	newP := fs.String("new", "", "path to the new schema version")
	fmts := addFormatFlag(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *oldP == "" || *newP == "" {
		fmt.Fprintln(os.Stderr, "compat: both --old and --new are required")
		return 2
	}
	f, err := parseFormat(*fmts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "compat:", err)
		return 2
	}
	oldData, err := readFile(*oldP)
	if err != nil {
		fmt.Fprintln(os.Stderr, "compat:", err)
		return 1
	}
	newData, err := readFile(*newP)
	if err != nil {
		fmt.Fprintln(os.Stderr, "compat:", err)
		return 1
	}
	oldS, err := parse.ParseSchema(oldData, parse.Options{Format: f, Origin: *oldP})
	if err != nil {
		fmt.Fprintf(os.Stderr, "compat: old document rejected (%s):\n", *oldP)
		render.ParseErrors(os.Stderr, err)
		return 1
	}
	newS, err := parse.ParseSchema(newData, parse.Options{Format: f, Origin: *newP})
	if err != nil {
		fmt.Fprintf(os.Stderr, "compat: new document rejected (%s):\n", *newP)
		render.ParseErrors(os.Stderr, err)
		return 1
	}
	rep := compat.Compare(oldS, newS)
	render.CompatReport(os.Stdout, rep)
	if rep.Verdict == compat.VerdictBreaking {
		return 1
	}
	return 0
}

func runLineage(args []string) int {
	fs := flag.NewFlagSet("lineage", flag.ContinueOnError)
	file := fs.String("file", "", "path to a catalog file")
	fmts := addFormatFlag(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *file == "" {
		fmt.Fprintln(os.Stderr, "lineage: --file is required")
		return 2
	}
	f, err := parseFormat(*fmts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lineage:", err)
		return 2
	}
	data, err := readFile(*file)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lineage:", err)
		return 1
	}
	cat, perr := parse.ParseCatalog(data, parse.Options{Format: f, Origin: *file})
	if perr != nil {
		fmt.Fprintf(os.Stderr, "lineage: document rejected (%s):\n", *file)
		render.ParseErrors(os.Stderr, perr)
		return 1
	}
	rep := lineage.Analyze(cat)
	render.LineageReport(os.Stdout, rep)
	if len(rep.Issues) > 0 {
		return 1
	}
	return 0
}
