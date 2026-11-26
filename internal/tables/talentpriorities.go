package tables

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// TalentPriorityRow is one row of talent_priorities: one entry of a spec's ranked key leveling
// talents list.
type TalentPriorityRow struct {
	Class      string `parquet:"class,dict"`
	Spec       string `parquet:"spec,dict"`
	Rank       int16  `parquet:"rank"`
	NodeID     int32  `parquet:"node_id"`
	TalentName string `parquet:"talent_name"`
	Reason     string `parquet:"reason"`
	SourceURL  string `parquet:"source_url,dict"`
	FetchedAt  string `parquet:"fetched_at,dict"`
}

// PrioritiesFile is the curated priorities file, kept beside curated/talents.
const PrioritiesFile = "talent_priorities.json"

// CuratedPriorities is curated/talent_priorities.json: the guides' key-talent lists by spec.
type CuratedPriorities struct {
	FetchedAt string            `json:"fetched_at"`
	Specs     []CuratedPrioSpec `json:"specs"`
}

// CuratedPrioSpec is one spec's list, best first. Reasons are written in our own words.
type CuratedPrioSpec struct {
	Class     string            `json:"class"`
	Spec      string            `json:"spec"`
	SourceURL string            `json:"source_url"`
	Talents   []CuratedPrioItem `json:"talents"`
}

// CuratedPrioItem is one ranked talent.
type CuratedPrioItem struct {
	NodeID int32  `json:"node_id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// LoadTalentPriorities reads PrioritiesFile from dir (the curated directory). A missing file
// is an empty list: no guide has been read yet.
func LoadTalentPriorities(dir string) (CuratedPriorities, error) {
	var c CuratedPriorities
	raw, err := os.ReadFile(filepath.Join(dir, PrioritiesFile))
	if os.IsNotExist(err) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	return c, json.Unmarshal(raw, &c)
}

// BuildTalentPriorities validates each entry against talent_nodes (the node exists, carries
// the name, has a reason) and flattens the lists into rows.
func BuildTalentPriorities(c CuratedPriorities, nodes []TalentNode) ([]TalentPriorityRow, error) {
	byID := map[int32]TalentNode{}
	for _, n := range nodes {
		byID[n.NodeID] = n
	}
	var errs []string
	rows := []TalentPriorityRow{}
	for _, s := range c.Specs {
		for i, t := range s.Talents {
			n, ok := byID[t.NodeID]
			switch {
			case !ok:
				errs = append(errs, fmt.Sprintf("%s %s: node %d is not in the talent tables", s.Class, s.Spec, t.NodeID))
			case n.Name == nil || *n.Name != t.Name:
				errs = append(errs, fmt.Sprintf("%s %s: node %d is named %q, not %q", s.Class, s.Spec, t.NodeID, deref(n.Name), t.Name))
			}
			if strings.TrimSpace(t.Reason) == "" {
				errs = append(errs, fmt.Sprintf("%s %s: %s has no reason", s.Class, s.Spec, t.Name))
			}
			rows = append(rows, TalentPriorityRow{Class: s.Class, Spec: s.Spec, Rank: int16(i + 1), NodeID: t.NodeID,
				TalentName: t.Name, Reason: t.Reason, SourceURL: s.SourceURL, FetchedAt: c.FetchedAt})
		}
	}
	if len(errs) > 0 {
		return nil, fmt.Errorf("talent priorities invalid:\n%s", strings.Join(errs, "\n"))
	}
	return rows, nil
}
