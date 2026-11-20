// Package manifest holds the dataset manifest model, its JSON Schema check and writer.
package manifest

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/WoW-Companion/data/schemas"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Source records where a table's data came from.
type Source struct {
	Name      string `json:"name"`
	Version   string `json:"version"`   // the source's version or the game build it came from
	Retrieved string `json:"retrieved"` // RFC 3339 timestamp
	License   string `json:"license"`
}

// Table is one Parquet file in the dataset.
type Table struct {
	Path     string   `json:"path"`
	Rows     int64    `json:"rows"`
	SHA256   string   `json:"sha256"`
	SortKeys []string `json:"sort_keys"`
	Sources  []Source `json:"sources"`
}

// Manifest describes one dataset version.
type Manifest struct {
	DatasetVersion string  `json:"dataset_version"`
	GameBuild      string  `json:"game_build"`
	Created        string  `json:"created"` // RFC 3339 timestamp
	Tables         []Table `json:"tables"`
}

func compile() (*jsonschema.Schema, error) {
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(schemas.Manifest))
	if err != nil {
		return nil, err
	}
	c := jsonschema.NewCompiler()
	c.AssertFormat()
	if err := c.AddResource("manifest.schema.json", doc); err != nil {
		return nil, err
	}
	return c.Compile("manifest.schema.json")
}

// Validate checks JSON-encoded manifest bytes against the schema and returns an error
// listing every violation.
func Validate(data []byte) error {
	sch, err := compile()
	if err != nil {
		return err
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("manifest is not valid JSON: %w", err)
	}
	verr := sch.Validate(inst)
	if verr == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(verr, &ve) {
		return verr
	}
	var lines []string
	collect(ve, &lines)
	sort.Strings(lines)
	return errors.New(strings.Join(lines, "; "))
}

func collect(e *jsonschema.ValidationError, out *[]string) {
	if len(e.Causes) == 0 {
		loc := "/" + strings.Join(e.InstanceLocation, "/")
		*out = append(*out, loc+": "+e.ErrorKind.LocalizedString(messagePrinter))
		return
	}
	for _, c := range e.Causes {
		collect(c, out)
	}
}

// Encode validates m and returns its JSON bytes; nothing is returned if it is invalid.
func Encode(m Manifest) ([]byte, error) {
	if m.Tables == nil {
		m.Tables = []Table{}
	}
	for i := range m.Tables {
		if m.Tables[i].SortKeys == nil {
			m.Tables[i].SortKeys = []string{}
		}
		if m.Tables[i].Sources == nil {
			m.Tables[i].Sources = []Source{}
		}
	}
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := Validate(b); err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

// Write validates m, then writes it to path; nothing is written if it is invalid.
func Write(m Manifest, path string) error {
	b, err := Encode(m)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

// Decode parses manifest JSON; call Validate first for schema errors.
func Decode(b []byte) (Manifest, error) {
	var m Manifest
	err := json.Unmarshal(b, &m)
	return m, err
}

// VerifyFile validates the manifest at path against the schema and checks that each
// listed file beside it exists with the recorded sha256.
func VerifyFile(path string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := Validate(b); err != nil {
		return err
	}
	m, err := Decode(b)
	if err != nil {
		return err
	}
	for _, t := range m.Tables {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(path), t.Path))
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		if got := hex.EncodeToString(sum[:]); got != t.SHA256 {
			return fmt.Errorf("%s: sha256 %s does not match manifest %s", t.Path, got, t.SHA256)
		}
	}
	return nil
}
