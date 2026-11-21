package schema

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/parquet-go/parquet-go"
)

// ColumnCoverage counts, per row, how many rows hold nothing in a column. A row is null
// when every value it carries in the column is null (an empty list is null) and zero when
// it is not null and every value is zero, false or an empty string.
type ColumnCoverage struct {
	Name       string
	Null, Zero int64
}

// TableCoverage is the coverage of one Parquet file.
type TableCoverage struct {
	Name    string
	Rows    int64
	Missing bool // the file is not in the dataset directory
	Columns []ColumnCoverage
}

func isZero(v parquet.Value) bool {
	switch v.Kind() {
	case parquet.Boolean:
		return !v.Boolean()
	case parquet.Int32:
		return v.Int32() == 0
	case parquet.Int64:
		return v.Int64() == 0
	case parquet.Float:
		return v.Float() == 0
	case parquet.Double:
		return v.Double() == 0
	case parquet.ByteArray, parquet.FixedLenByteArray:
		return len(v.ByteArray()) == 0
	}
	return false
}

// columnName drops the list wrapper names, so stats.list.element.value reads stats.value.
func columnName(path []string) string {
	var out []string
	for _, p := range path {
		if p != "list" && p != "element" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ".")
}

// CoverageOf counts null and zero rows per column of one Parquet file's bytes.
func CoverageOf(name string, b []byte) (TableCoverage, error) {
	tc := TableCoverage{Name: name}
	f, err := parquet.OpenFile(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		return tc, err
	}
	leaves := f.Schema().Columns()
	for _, p := range leaves {
		tc.Columns = append(tc.Columns, ColumnCoverage{Name: columnName(p)})
	}
	r := parquet.NewReader(f)
	defer r.Close()
	rows := make([]parquet.Row, 256)
	nulls := make([]bool, len(leaves)) // per row: every value of the column so far is null
	zeros := make([]bool, len(leaves))
	for {
		n, err := r.ReadRows(rows)
		for _, row := range rows[:n] {
			for i := range leaves {
				nulls[i], zeros[i] = true, true
			}
			for _, v := range row {
				i := v.Column()
				if !v.IsNull() {
					nulls[i] = false
					if !isZero(v) {
						zeros[i] = false
					}
				}
			}
			for i := range leaves {
				if nulls[i] {
					tc.Columns[i].Null++
				} else if zeros[i] {
					tc.Columns[i].Zero++
				}
			}
			tc.Rows++
		}
		if err == io.EOF {
			return tc, nil
		}
		if err != nil {
			return tc, err
		}
	}
}

// Coverage reads every registered table from a dataset directory.
func Coverage(dir string) ([]TableCoverage, error) {
	var out []TableCoverage
	for _, t := range Tables {
		b, err := os.ReadFile(filepath.Join(dir, t.File()))
		if os.IsNotExist(err) {
			out = append(out, TableCoverage{Name: t.Name, Missing: true})
			continue
		}
		if err != nil {
			return nil, err
		}
		tc, err := CoverageOf(t.Name, b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", t.File(), err)
		}
		out = append(out, tc)
	}
	return out, nil
}

func pct(n, rows int64) string {
	if rows == 0 {
		return "n/a"
	}
	return fmt.Sprintf("%.1f%%", 100*float64(n)/float64(rows))
}

// CoverageMarkdown renders the coverage report.
func CoverageMarkdown(build string, tcs []TableCoverage) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Dataset coverage: %s\n\nShare of rows that are null (including an empty list) or zero (false, empty string) in each column. Null and zero are disjoint.\n\n", build)
	b.WriteString("| Table | Rows |\n| --- | ---: |\n")
	for _, t := range tcs {
		if t.Missing {
			fmt.Fprintf(&b, "| %s | missing |\n", t.Name)
		} else {
			fmt.Fprintf(&b, "| %s | %d |\n", t.Name, t.Rows)
		}
	}
	for _, t := range tcs {
		if t.Missing {
			continue
		}
		fmt.Fprintf(&b, "\n## %s\n\n%d rows.\n\n| Column | Null | Zero |\n| --- | ---: | ---: |\n", t.Name, t.Rows)
		for _, c := range t.Columns {
			fmt.Fprintf(&b, "| `%s` | %s | %s |\n", c.Name, pct(c.Null, t.Rows), pct(c.Zero, t.Rows))
		}
	}
	return b.String()
}
