package everloot

import (
	"slices"
)

// EncounterRecord is one public record of kind encounter: an install saw a dungeon boss
// encounter end. EncounterID is the client's DungeonEncounter id; the name is in the
// install's own language, so it is never kept.
type EncounterRecord struct {
	InstallID     string `json:"installId"`
	EncounterID   int32  `json:"encounterId"`
	InstanceMapID int32  `json:"instanceMapId"`
	Success       bool   `json:"success"`
}

// EntranceRecord is one public record of kind instance_entrance: the open-world UiMap an
// install stood on when it entered the instance.
type EntranceRecord struct {
	InstallID     string `json:"installId"`
	InstanceMapID int32  `json:"instanceMapId"`
	MapID         int32  `json:"mapId"`
}

// EncounterKey names one boss encounter of one instance map.
type EncounterKey struct{ MapID, EncounterID int32 }

// BuildEncounterObs returns, for each encounter at least MinReporters distinct installs
// killed (success true), how many installs did.
func BuildEncounterObs(c Corpus) map[EncounterKey]int32 {
	seen := map[EncounterKey]map[string]struct{}{}
	for _, r := range c.Encounters {
		if !r.Success || r.InstallID == "" || r.EncounterID <= 0 || r.InstanceMapID <= 0 {
			continue
		}
		k := EncounterKey{r.InstanceMapID, r.EncounterID}
		if seen[k] == nil {
			seen[k] = map[string]struct{}{}
		}
		seen[k][r.InstallID] = struct{}{}
	}
	out := map[EncounterKey]int32{}
	for k, s := range seen {
		if len(s) >= MinReporters {
			out[k] = int32(len(s))
		}
	}
	return out
}

// EntranceZone is the UiMap an instance's entrance stands on and how many installs agreed.
type EntranceZone struct {
	UIMapID   int32
	Reporters int32
}

// BuildEntranceZones keeps an instance's entrance zone only when at least MinReporters
// distinct installs report it and no install reports a different zone for that instance:
// the records give the map the player stood on at entry, which is wrong when someone queues
// from a city, so any disagreement drops the instance rather than guess.
func BuildEntranceZones(c Corpus) map[int32]EntranceZone {
	by := map[int32]map[int32]map[string]struct{}{}
	for _, r := range c.Entrances {
		if r.InstallID == "" || r.InstanceMapID <= 0 || r.MapID <= 0 {
			continue
		}
		if by[r.InstanceMapID] == nil {
			by[r.InstanceMapID] = map[int32]map[string]struct{}{}
		}
		if by[r.InstanceMapID][r.MapID] == nil {
			by[r.InstanceMapID][r.MapID] = map[string]struct{}{}
		}
		by[r.InstanceMapID][r.MapID][r.InstallID] = struct{}{}
	}
	out := map[int32]EntranceZone{}
	for inst, zones := range by {
		ids := make([]int32, 0, len(zones))
		for z := range zones {
			ids = append(ids, z)
		}
		slices.Sort(ids)
		if len(ids) == 1 && len(zones[ids[0]]) >= MinReporters {
			out[inst] = EntranceZone{UIMapID: ids[0], Reporters: int32(len(zones[ids[0]]))}
		}
	}
	return out
}
