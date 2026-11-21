package bench

import (
	"fmt"
	"strings"
	"time"
)

// Markdown renders the results as three tables: layout, point lookups, 100-row range reads.
func Markdown(build string, cfg Config, ts []Table) string {
	var b strings.Builder
	var rows, size int64
	for _, t := range ts {
		rows += t.Rows
		size += t.Size
	}
	fmt.Fprintf(&b, "## Bench: %s\n\n%d tables, %d rows, %d bytes. %d local and %d HTTP lookups per case, %s added to every HTTP request.\n\n",
		build, len(ts), rows, size, cfg.Local, cfg.HTTP, cfg.Latency.Round(time.Millisecond))
	b.WriteString("### Layout\n\n| table | rows | bytes | row groups | key | cols | dict cols | Bloom columns | Bloom bytes |\n|---|---|---|---|---|---|---|---|---|\n")
	for _, t := range ts {
		bloom := "-"
		if len(t.Bloom) > 0 {
			bloom = strings.Join(t.Bloom, ", ")
		}
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %s | %d | %d | %s | %d |\n", t.Name, t.Rows, t.Size, t.RowGroups, t.Key, t.Cols, t.DictCols, bloom, t.BloomSize)
	}
	for _, op := range []struct {
		title string
		get   func(Table) Case
	}{
		{"Point lookup by key", func(t Table) Case { return t.Point }},
		{fmt.Sprintf("Range read of %d rows by key", RangeRows), func(t Table) Case { return t.Range }},
	} {
		fmt.Fprintf(&b, "\n### %s (ms p50 / p95; bytes and requests per lookup)\n\n", op.title)
		b.WriteString("| table | local cold | local warm | http cold | http warm | local cold bytes | http cold bytes | http cold reqs | http warm bytes | http warm reqs |\n|---|---|---|---|---|---|---|---|---|---|\n")
		for _, t := range ts {
			c := op.get(t)
			fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %d | %d | %.1f | %d | %.1f |\n", t.Name,
				ms(c.LocalCold), ms(c.LocalWarm), ms(c.HTTPCold), ms(c.HTTPWarm),
				c.LocalCold.Bytes, c.HTTPCold.Bytes, c.HTTPCold.Reqs, c.HTTPWarm.Bytes, c.HTTPWarm.Reqs)
		}
	}
	return b.String()
}

func ms(s Stat) string { return fmt.Sprintf("%.2f / %.2f", s.P50, s.P95) }
