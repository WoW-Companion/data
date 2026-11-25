package everloot

import (
	"fmt"
	"path/filepath"
)

// Status is the outcome of fetching one Everloot source.
type Status struct {
	Source string
	Err    error // nil when the source was fetched or read from cache
}

// Line is the status line printed for the source.
func (s Status) Line() string {
	if s.Err == nil {
		return fmt.Sprintf("everloot %s: ok", s.Source)
	}
	return fmt.Sprintf("everloot %s: UNAVAILABLE, the tables built from it are empty in this build: %v", s.Source, s.Err)
}

// FetchAll fetches the drop rates, the corpus and the disenchant rollup into cacheDir. A
// source that cannot be fetched (rate limited, down) is reported in its Status and left out
// of the cache, so the build reads it as empty: a busy third-party source never fails the
// dataset build. Nothing partial is cached.
func FetchAll(baseURL, bulkBase, cacheDir, retrieved, build string) (Snapshot, Corpus, Disenchant, []Status) {
	var st []Status
	var ex *Export
	if bulkBase != "" {
		var err error
		if ex, err = FetchExport(bulkBase, cacheDir); err != nil {
			ex = nil
			st = append(st, Status{"data files export (falling back to the API)", err})
		} else {
			st = append(st, Status{fmt.Sprintf("data files export %s dataAsOf %s", ex.Path, ex.Index.DataAsOf), nil})
		}
	}
	ev, err := FetchWith(baseURL, cacheDir, retrieved, ex)
	if err != nil {
		ev = Snapshot{}
	}
	st = append(st, Status{"drops and edges (" + filepath.Join(cacheDir, CacheFile) + ")", err})
	cor, err := FetchCorpusWith(baseURL, cacheDir, retrieved, build, ex)
	if err != nil {
		cor = Corpus{}
	}
	st = append(st, Status{"corpus (quests, vendors, encounters, entrances, trainers)", err})
	de, err := FetchDisenchant(baseURL, cacheDir, retrieved)
	if err != nil {
		de = Disenchant{}
	}
	st = append(st, Status{"disenchant rollup", err})
	return ev, cor, de, st
}
