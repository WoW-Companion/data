package manifest

import (
	"encoding/json"
	"strings"
	"testing"
)

const valid = `{
  "dataset_version": "1.0.0", "game_build": "1.0.0.12345", "created": "2026-11-20T00:00:00Z",
  "tables": [{"path": "items.parquet", "rows": 3, "sha256": "` + "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef" + `",
    "sort_keys": ["id"],
    "sources": [{"name": "n", "version": "1", "retrieved": "2026-11-20T00:00:00Z", "license": "l"}]}]
}`

// mutate decodes valid, applies f to the root object and re-encodes it.
func mutate(t *testing.T, f func(root, table, source map[string]any)) []byte {
	t.Helper()
	var root map[string]any
	if err := json.Unmarshal([]byte(valid), &root); err != nil {
		t.Fatal(err)
	}
	table := root["tables"].([]any)[0].(map[string]any)
	source := table["sources"].([]any)[0].(map[string]any)
	f(root, table, source)
	b, _ := json.Marshal(root)
	return b
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name   string
		edit   func(root, table, source map[string]any)
		wantIn string // empty means valid
	}{
		{"valid", func(_, _, _ map[string]any) {}, ""},
		{"no dataset_version", func(r, _, _ map[string]any) { delete(r, "dataset_version") }, "dataset_version"},
		{"no game_build", func(r, _, _ map[string]any) { delete(r, "game_build") }, "game_build"},
		{"no created", func(r, _, _ map[string]any) { delete(r, "created") }, "created"},
		{"no tables", func(r, _, _ map[string]any) { delete(r, "tables") }, "tables"},
		{"no path", func(_, tb, _ map[string]any) { delete(tb, "path") }, "path"},
		{"no rows", func(_, tb, _ map[string]any) { delete(tb, "rows") }, "rows"},
		{"no sha256", func(_, tb, _ map[string]any) { delete(tb, "sha256") }, "sha256"},
		{"no sort_keys", func(_, tb, _ map[string]any) { delete(tb, "sort_keys") }, "sort_keys"},
		{"no sources", func(_, tb, _ map[string]any) { delete(tb, "sources") }, "sources"},
		{"no source license", func(_, _, s map[string]any) { delete(s, "license") }, "license"},
		{"bad sha256", func(_, tb, _ map[string]any) { tb["sha256"] = "xyz" }, "sha256"},
		{"negative rows", func(_, tb, _ map[string]any) { tb["rows"] = -1 }, "rows"},
		{"empty sources", func(_, tb, _ map[string]any) { tb["sources"] = []any{} }, "sources"},
		{"bad created", func(r, _, _ map[string]any) { r["created"] = "yesterday" }, "created"},
		{"extra key", func(r, _, _ map[string]any) { r["extra"] = 1 }, "extra"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(mutate(t, c.edit))
			switch {
			case c.wantIn == "" && err != nil:
				t.Fatalf("want valid, got %v", err)
			case c.wantIn != "" && err == nil:
				t.Fatal("want error, got nil")
			case c.wantIn != "" && !strings.Contains(err.Error(), c.wantIn):
				t.Fatalf("error %q does not mention %q", err, c.wantIn)
			}
		})
	}
}

func TestEncodeRefusesInvalid(t *testing.T) {
	if _, err := Encode(Manifest{}); err == nil {
		t.Fatal("empty manifest encoded")
	}
}
