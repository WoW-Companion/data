// Command wowc-data fetches client tables and builds the WoW: Forever dataset.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/WoW-Companion/data/internal/bench"
	"github.com/WoW-Companion/data/internal/carry"
	"github.com/WoW-Companion/data/internal/diff"
	"github.com/WoW-Companion/data/internal/everloot"
	"github.com/WoW-Companion/data/internal/fetch"
	"github.com/WoW-Companion/data/internal/history"
	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/WoW-Companion/data/internal/names"
	"github.com/WoW-Companion/data/internal/release"
	"github.com/WoW-Companion/data/internal/schema"
	"github.com/WoW-Companion/data/internal/tables"
)

const usage = `usage: wowc-data <command> [flags]

commands:
  fetch            download client tables into the cache (once per build)
  build            build Parquet tables and manifest.json into dist/<build>/
  verify-manifest  check manifest.json against the schema and the files it lists
  schema           write the dataset schema reference (docs/schema.md)
  coverage         print per-table row counts and null or zero shares of a built dataset
  diff             print what changed between two dataset directories, as Markdown
  bench            measure each table of a built dataset: size, layout, lookup time and bytes read
  release-notes    print the release notes of a built dataset, with what changed since the previous release
  history          add a built dataset's revision to the build's kept revisions (revisions-<build>.json) and name the ones to prune
  release-check    print the latest Forever build, whether its dataset release exists and whether its revision is current
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
	case "schema":
		err = runSchema(os.Args[2:])
	case "coverage":
		err = runCoverage(os.Args[2:])
	case "diff":
		err = runDiff(os.Args[2:])
	case "bench":
		err = runBench(os.Args[2:])
	case "release-notes":
		err = runReleaseNotes(os.Args[2:])
	case "history":
		err = runHistory(os.Args[2:])
	case "release-check":
		err = runReleaseCheck(os.Args[2:])
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
	loot := fs.Bool("loot", false, "also fetch Everloot loot windows (50 pages: more than 30 requests an hour; for gathering and fishing). Always on when EVERLOOT_API_KEY is set")
	fs.Parse(args)
	everloot.FetchLoot = *loot
	prov, err := fetch.Build(fetch.BaseURL, *build, *cache, fetch.Tables)
	if err != nil {
		return err
	}
	ev, cor, de, status := everloot.FetchAll(everloot.BaseURL, everloot.BulkBase, filepath.Join(*cache, "everloot"), time.Now().UTC().Format("2006-01-02T15:04:05Z"), *build)
	for _, st := range status {
		fmt.Println(st.Line())
	}
	fmt.Printf("cached %d disenchant bands, %d trainer records, %d loot records\n", len(de.Bands), len(cor.Trainers), len(cor.Loots))
	fmt.Printf("cached %d Everloot quest records and %d vendor records (data as of %s)\n", len(cor.Quests), len(cor.Vendors), cor.DataAsOf)
	fmt.Printf("cached %d tables for build %s in %s\n", len(prov.Tables), prov.Build, filepath.Join(*cache, *build))
	fmt.Printf("cached %d Everloot drop rates (data as of %s)\n", len(ev.Rates), ev.DataAsOf)
	return nil
}

func runBuild(args []string) error {
	fs := flag.NewFlagSet("build", flag.ExitOnError)
	build := fs.String("build", fetch.DefaultBuild, "game build")
	cache := fs.String("cache", "dist/cache", "cache directory")
	out := fs.String("out", "dist", "output root; files go to <out>/<build>/")
	talents := fs.String("talents", "curated/talents", "directory of curated talent builds")
	instances := fs.String("instances", "curated/instances.yaml", "curated instance level ranges and entrance zones")
	professions := fs.String("professions", "curated/professions.yaml", "curated profession notes")
	repo := fs.String("repo", os.Getenv("GITHUB_REPOSITORY"), "owner/name of the releasing repository: earlier releases feed carry-forward and the shrink guard; empty skips both")
	api := fs.String("api", "https://api.github.com", "GitHub API base URL")
	download := fs.String("download", "https://github.com", "public release download base URL")
	allowFile := fs.String("allow-shrink", release.AllowShrinkFile, "committed list of tables allowed to shrink")
	fs.Parse(args)
	dir := filepath.Join(*out, *build)
	rev, err := release.Revision(".")
	if err != nil {
		return err
	}
	var finder *carry.Finder
	var prev tables.Previous
	if *repo != "" {
		finder = &carry.Finder{API: *api, Download: *download, Repo: *repo, Build: *build, Token: os.Getenv("GITHUB_TOKEN"), UserAgent: "wowc-data",
			Log: func(f string, a ...any) { fmt.Printf(f+"\n", a...) }}
		prev = finder
	} else {
		fmt.Println("no repository (GITHUB_REPOSITORY): carry-forward and the shrink guard are skipped")
	}
	m, err := tables.BuildDatasetFrom(filepath.Join(*cache, *build), filepath.Join(*cache, "everloot", everloot.CacheFile), *talents, *instances, *professions, dir, time.Now().UTC().Format("2006-01-02T15:04:05Z"), rev, prev)
	if err != nil {
		return err
	}
	if finder != nil {
		old, ok, err := finder.Baseline()
		if err != nil {
			return fmt.Errorf("shrink guard: %w", err)
		}
		if ok {
			allow, err := release.LoadAllowShrink(*allowFile)
			if err != nil {
				return err
			}
			if err := release.CheckShrink(old, m, allow); err != nil {
				return err
			}
			fmt.Printf("shrink guard: no table shrank against %s\n", old.Revision)
		}
	}
	for _, t := range m.Tables {
		fmt.Printf("%s: %d rows\n", t.Path, t.Rows)
	}
	// A warning, never a failure: the coverage report lists the same ids.
	if gaps, err := schema.ItemNameGaps(dir); err != nil {
		fmt.Printf("WARNING: item name coverage not checked: %v\n", err)
	} else if len(gaps) > 0 {
		fmt.Printf("WARNING: %d referenced item ids have no name:%s", gaps.Total(), gaps.Markdown()[len("\n## Item name coverage\n\n"):])
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

func runSchema(args []string) error {
	fs := flag.NewFlagSet("schema", flag.ExitOnError)
	out := fs.String("out", "docs/schema.md", "file to write")
	fs.Parse(args)
	return os.WriteFile(*out, []byte(schema.Markdown()), 0o644)
}

func runCoverage(args []string) error {
	fs := flag.NewFlagSet("coverage", flag.ExitOnError)
	asJSON := fs.Bool("json", false, "print the stats as JSON, the form diff reads")
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: wowc-data coverage <dataset dir>")
	}
	dir := filepath.Clean(fs.Arg(0))
	tcs, err := schema.Coverage(dir)
	if err != nil {
		return err
	}
	if *asJSON {
		b, err := schema.CoverageJSON(tcs)
		if err != nil {
			return err
		}
		fmt.Print(string(b))
		return nil
	}
	fmt.Print(schema.CoverageMarkdown(filepath.Base(dir), tcs))
	gaps, err := schema.ItemNameGaps(dir)
	if err != nil {
		return err
	}
	fmt.Print(gaps.Markdown())
	return nil
}

func runDiff(args []string) error {
	fs := flag.NewFlagSet("diff", flag.ExitOnError)
	fs.Parse(args)
	if fs.NArg() != 2 {
		return fmt.Errorf("usage: wowc-data diff <old dir> <new dir>")
	}
	old, err := diff.Load(fs.Arg(0))
	if err != nil {
		return err
	}
	cur, err := diff.Load(fs.Arg(1))
	if err != nil {
		return err
	}
	fmt.Print(diff.Markdown(old, cur))
	return nil
}

func runBench(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ExitOnError)
	cfg := bench.Config{}
	fs.IntVar(&cfg.Local, "n", 200, "lookups per local case")
	fs.IntVar(&cfg.HTTP, "http-n", 20, "lookups per HTTP case")
	fs.DurationVar(&cfg.Latency, "latency", 20*time.Millisecond, "delay added to every HTTP request")
	fs.Parse(args)
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: wowc-data bench [-n N] [-http-n N] [-latency D] <dataset dir>")
	}
	dir := filepath.Clean(fs.Arg(0))
	ts, err := bench.Run(dir, cfg)
	if err != nil {
		return err
	}
	fmt.Print(bench.Markdown(filepath.Base(dir), cfg, ts))
	if _, err := os.Stat(filepath.Join(dir, names.File)); err == nil {
		nc, err := bench.Names(dir, cfg)
		if err != nil {
			return err
		}
		fmt.Print(bench.NamesMarkdown(nc, cfg))
	}
	return nil
}

// runReleaseCheck prints build=<build> and exists=<true|false> lines, the format of
// $GITHUB_OUTPUT, and appends them there when it is set. GITHUB_TOKEN and
// GITHUB_REPOSITORY come from the environment.
func runReleaseCheck(args []string) error {
	fs := flag.NewFlagSet("release-check", flag.ExitOnError)
	repo := fs.String("repo", os.Getenv("GITHUB_REPOSITORY"), "owner/name of the releasing repository")
	api := fs.String("api", "https://api.github.com", "GitHub API base URL")
	fs.Parse(args)
	if *repo == "" {
		return fmt.Errorf("release-check: -repo or GITHUB_REPOSITORY required")
	}
	build, err := release.LatestBuild(fetch.BaseURL, fetch.Product, fetch.UserAgent)
	if err != nil {
		return err
	}
	exists, err := release.Exists(*api, *repo, release.Tag(build), os.Getenv("GITHUB_TOKEN"))
	if err != nil {
		return err
	}
	rev, err := release.Revision(".")
	if err != nil {
		return err
	}
	published := ""
	if exists {
		if published, err = release.PublishedRevision(*api, *repo, release.Tag(build), build, os.Getenv("GITHUB_TOKEN")); err != nil {
			return err
		}
	}
	out := fmt.Sprintf("build=%s\nexists=%t\nrevision=%s\npublished=%s\nrevision_differs=%t\npublish=%t\n",
		build, exists, rev, published, exists && published != rev, release.NeedsPublish(exists, published, rev))
	fmt.Print(out)
	if p := os.Getenv("GITHUB_OUTPUT"); p != "" {
		f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		if _, err := f.WriteString(out); err != nil {
			return err
		}
	}
	return nil
}

// runReleaseNotes prints the notes of the dataset in <dir>. With -prev-tag and -prev-dir
// (the previous release's manifest.json and, if it has one, stats.json) they open with
// the diff; without, they say it is the first release.
func runReleaseNotes(args []string) error {
	fs := flag.NewFlagSet("release-notes", flag.ExitOnError)
	prevTag := fs.String("prev-tag", "", "tag of the previous dataset release; empty for the first")
	prevDir := fs.String("prev-dir", "", "directory holding the previous release's manifest.json and stats.json")
	fs.Parse(args)
	if fs.NArg() != 1 || (*prevTag != "") != (*prevDir != "") {
		return fmt.Errorf("usage: wowc-data release-notes [-prev-tag TAG -prev-dir DIR] <dataset dir>")
	}
	cur, err := diff.Load(fs.Arg(0))
	if err != nil {
		return err
	}
	changes := ""
	if *prevTag != "" {
		old, err := diff.Load(*prevDir)
		if err != nil {
			return err
		}
		changes = diff.Markdown(old, cur)
	}
	fmt.Print(release.NotesAt(cur.Manifest.GameBuild, *prevTag, changes, cur.Manifest.Revision, cur.Manifest.Created[:min(10, len(cur.Manifest.Created))]))
	return nil
}

// runHistory adds the built dataset's revision to the build's kept revisions: it reads the
// release's current revisions-<build>.json (-prev, may be absent), writes the new one (-out)
// and prints "revision <rev>" then one "prune <rev>" line for each revision that fell out of
// the last 10, for the publish script to name and delete assets by.
func runHistory(args []string) error {
	fs := flag.NewFlagSet("history", flag.ExitOnError)
	prev := fs.String("prev", "", "the release's current revisions file; absent or empty means none yet")
	out := fs.String("out", "", "revisions file to write")
	fs.Parse(args)
	if fs.NArg() != 1 || *out == "" {
		return fmt.Errorf("usage: wowc-data history -out <revisions-build.json> [-prev <file>] <dataset dir>")
	}
	mb, err := os.ReadFile(filepath.Join(fs.Arg(0), "manifest.json"))
	if err != nil {
		return err
	}
	m, err := manifest.Decode(mb)
	if err != nil {
		return err
	}
	if m.Revision == "" {
		return fmt.Errorf("the manifest has no revision")
	}
	cur := history.File{Build: m.GameBuild}
	if *prev != "" {
		if b, err := os.ReadFile(*prev); err == nil {
			if cur, err = history.Parse(b); err != nil {
				return err
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	cur.Build = m.GameBuild
	next, pruned := cur.Add(history.FromManifest(m), history.Keep)
	b, err := next.Marshal()
	if err != nil {
		return err
	}
	if err := os.WriteFile(*out, b, 0o644); err != nil {
		return err
	}
	fmt.Println("revision", m.Revision)
	for _, r := range pruned {
		fmt.Println("prune", r)
	}
	return nil
}
