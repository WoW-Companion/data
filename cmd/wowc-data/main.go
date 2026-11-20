// Command wowc-data fetches client tables and builds the WoW: Forever dataset.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/WoW-Companion/data/internal/fetch"
	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/WoW-Companion/data/internal/tables"
)

const usage = `usage: wowc-data <command> [flags]

commands:
  fetch            download client tables into the cache (once per build)
  build            build Parquet tables and manifest.json into dist/<build>/
  verify-manifest  check manifest.json against the schema and the files it lists
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "fetch":
		err = runFetch(os.Args[2:])
	case "build":
		err = runBuild(os.Args[2:])
	case "verify-manifest":
		err = runVerify(os.Args[2:])
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "wowc-data:", err)
		os.Exit(1)
	}
}

func runFetch(args []string) error {
	fs := flag.NewFlagSet("fetch", flag.ExitOnError)
	build := fs.String("build", fetch.DefaultBuild, "game build")
	cache := fs.String("cache", "dist/cache", "cache directory")
	fs.Parse(args)
	prov, err := fetch.Build(fetch.BaseURL, *build, *cache, fetch.Tables)
	if err != nil {
		return err
	}
	fmt.Printf("cached %d tables for build %s in %s\n", len(prov.Tables), prov.Build, filepath.Join(*cache, *build))
	return nil
}

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	build := fs.String("build", fetch.DefaultBuild, "game build")
	cache := fs.String("cache", "dist/cache", "cache directory")
	out := fs.String("out", "dist", "output root; files go to <out>/<build>/")
	fs.Parse(args)
	dir := filepath.Join(*out, *build)
	m, err := tables.BuildDataset(filepath.Join(*cache, *build), dir, time.Now().UTC().Format("2006-01-02T15:04:05Z"))
	if err != nil {
		return err
	}
	for _, t := range m.Tables {
		fmt.Printf("%s: %d rows\n", t.Path, t.Rows)
	}
	return nil
}

func runVerify(args []string) error {
	fs := flag.NewFlagSet("verify-manifest", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: wowc-data verify-manifest <manifest.json | dataset dir>")
	}
	path := fs.Arg(0)
	if st, err := os.Stat(path); err == nil && st.IsDir() {
		path = filepath.Join(path, "manifest.json")
	}
	if err := manifest.VerifyFile(path); err != nil {
		return err
	}
	fmt.Println("manifest ok:", path)
	return nil
}
