package fetch

import (
	"reflect"
	"testing"
)

func TestTableURL(t *testing.T) {
	if got, want := TableURL("https://x.test", "Item", "1.2.3.4"), "https://x.test/db2/Item/csv?version=1.2.3.4"; got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestMakeProvenance(t *testing.T) {
	p := MakeProvenance("1.2.3.4", "2026-11-20T00:00:00Z", []string{"B", "A"})
	if !reflect.DeepEqual(p.Tables, []string{"A", "B"}) || p.Source != "wago.tools" || p.Build != "1.2.3.4" || p.Credit == "" || p.License == "" {
		t.Fatalf("%+v", p)
	}
}
