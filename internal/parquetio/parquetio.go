// Package parquetio writes the dataset's Parquet files: zstd, sorted by primary key,
// Bloom filters on id columns, one row group size, provenance in key-value metadata.
package parquetio

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"

	"github.com/WoW-Companion/data/internal/manifest"
	"github.com/WoW-Companion/data/internal/provenance"
	"github.com/parquet-go/parquet-go"
	"github.com/parquet-go/parquet-go/compress/zstd"
)

// DefaultRowGroup is the row group size adopted for the parquet-go reader (docs/spikes/parquet-layout.md).
const DefaultRowGroup = 4096

// Options controls one file's layout.
type Options struct {
	RowGroup int      // rows per row group; DefaultRowGroup when zero
	Bloom    []string // columns that get a Bloom filter
	Sort     []string // columns the rows are sorted by, recorded in the file; rows must already be in this order
	Sources  []manifest.Source
}

// Encode serialises rows into Parquet bytes.
func Encode[T any](rows []T, o Options) ([]byte, error) {
	if o.RowGroup == 0 {
		o.RowGroup = DefaultRowGroup
	}
	opts := []parquet.WriterOption{
		parquet.Compression(&zstd.Codec{}),
		parquet.MaxRowsPerRowGroup(int64(o.RowGroup)),
	}
	if len(o.Bloom) > 0 {
		var f []parquet.BloomFilterColumn
		for _, c := range o.Bloom {
			f = append(f, parquet.SplitBlockFilter(10, c))
		}
		opts = append(opts, parquet.BloomFilters(f...))
	}
	if len(o.Sort) > 0 {
		var cols []parquet.SortingColumn
		for _, c := range o.Sort {
			cols = append(cols, parquet.Ascending(c))
		}
		opts = append(opts, parquet.SortingWriterConfig(parquet.SortingColumns(cols...)))
	}
	if o.Sources != nil {
		v, err := provenance.Encode(o.Sources)
		if err != nil {
			return nil, err
		}
		opts = append(opts, parquet.KeyValueMetadata(provenance.Key, v))
	}
	var buf bytes.Buffer
	w := parquet.NewGenericWriter[T](&buf, opts...)
	// Write one row group's worth at a time so each group is cut at exactly RowGroup rows.
	for i := 0; i < len(rows); i += o.RowGroup {
		end := min(i+o.RowGroup, len(rows))
		if _, err := w.Write(rows[i:end]); err != nil {
			return nil, err
		}
		if err := w.Flush(); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// WriteFile encodes rows to path and returns the row count and the file's sha256.
func WriteFile[T any](path string, rows []T, o Options) (int64, string, error) {
	b, err := Encode(rows, o)
	if err != nil {
		return 0, "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return 0, "", err
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return 0, "", err
	}
	sum := sha256.Sum256(b)
	return int64(len(rows)), hex.EncodeToString(sum[:]), nil
}

// ReadSources returns the sources stamped into a Parquet file's metadata.
func ReadSources(b []byte) ([]manifest.Source, error) {
	f, err := parquet.OpenFile(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return nil, err
	}
	v, _ := f.Lookup(provenance.Key)
	return provenance.Decode(v)
}
