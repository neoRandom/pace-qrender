// qrender turns the JSON question records produced by the conversion agent
// into Markdown question files.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, stdout, stderr io.Writer) int {
	var o options
	fl := flag.NewFlagSet("qrender", flag.ContinueOnError)
	fl.SetOutput(stderr)
	fl.StringVar(&o.out, "o", "md", "output directory for the Markdown files")
	fl.BoolVar(&o.random, "random", false, "random identifiers instead of a hash of source+slug")
	fl.IntVar(&o.hashLen, "hash-len", 6, "identifier length in hex characters (4 to 64)")
	fl.BoolVar(&o.strict, "strict", false, "treat warnings (including PLACEHOLDER images) as errors")
	fl.BoolVar(&o.partial, "partial", false, "write the valid questions even if others have errors")
	fl.StringVar(&o.mapFile, "map", "", "write a TSV mapping JSON file to Markdown file to this path")
	fl.Usage = func() {
		fmt.Fprintln(stderr, "usage: qrender [flags] <questions.zip | folder containing questions/ and images/>")
		fl.PrintDefaults()
	}
	if err := fl.Parse(args); err != nil {
		return 2
	}
	if fl.NArg() != 1 || o.hashLen < 4 || o.hashLen > 64 {
		fl.Usage()
		return 2
	}

	fsys, closeFn, err := openInput(fl.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	defer closeFn()

	items, diags := build(fsys, o)
	diags = append(diags, assignNames(items, o)...)

	failed := map[string]bool{}
	warnings := 0
	for _, d := range diags {
		if d.warn && !o.strict {
			warnings++
			fmt.Fprintf(stderr, "warning %s: %s\n", d.file, d.msg)
			continue
		}
		failed[d.file] = true
		fmt.Fprintf(stderr, "ERROR   %s: %s\n", d.file, d.msg)
	}

	var ok []*item
	for _, it := range items {
		if !failed[it.jsonName] {
			ok = append(ok, it)
		}
	}
	if len(failed) > 0 && !o.partial {
		fmt.Fprintf(stderr, "FAILED: %d file(s) with errors; nothing written (use -partial to write the valid ones)\n", len(failed))
		return 1
	}

	if err := os.MkdirAll(o.out, 0o755); err != nil {
		fmt.Fprintln(stderr, "error:", err)
		return 2
	}
	placeholders := 0
	var tsv strings.Builder
	for _, it := range ok {
		if err := os.WriteFile(filepath.Join(o.out, it.mdName), []byte(it.body), 0o644); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 2
		}
		placeholders += it.placeholders
		fmt.Fprintf(stdout, "%s -> %s\n", it.jsonName, it.mdName)
		fmt.Fprintf(&tsv, "%s\t%s\n", it.jsonName, it.mdName)
	}
	if o.mapFile != "" {
		if err := os.WriteFile(o.mapFile, []byte(tsv.String()), 0o644); err != nil {
			fmt.Fprintln(stderr, "error:", err)
			return 2
		}
	}
	fmt.Fprintf(stdout, "rendered %d file(s), %d PLACEHOLDER image(s), %d warning(s), %d failed\n", len(ok), placeholders, warnings, len(failed))
	if len(failed) > 0 {
		return 1
	}
	return 0
}
