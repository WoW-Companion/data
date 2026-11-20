// Package provenance stamps and reads the sources recorded in a Parquet file's
// key-value metadata.
package provenance

import (
	"encoding/json"

	"github.com/WoW-Companion/data/internal/manifest"
)

// Key is the Parquet key-value metadata key holding the JSON-encoded sources.
const Key = "wowc.sources"

// Encode returns the metadata value for sources.
func Encode(sources []manifest.Source) (string, error) {
	b, err := json.Marshal(sources)
	return string(b), err
}

// Decode reads the sources back from a metadata value; an empty value is no sources.
func Decode(value string) ([]manifest.Source, error) {
	if value == "" {
		return nil, nil
	}
	var s []manifest.Source
	err := json.Unmarshal([]byte(value), &s)
	return s, err
}
