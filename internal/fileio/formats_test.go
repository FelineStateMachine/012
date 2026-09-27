package fileio

import "testing"

// TestFormats checks every row of the formats table is complete, since
// the UI's lists and Import and Export all come from it.
func TestFormats(t *testing.T) {
	for i, k := range Kinds() {
		f := k.format()
		if f.kind != k || k != Kind(i+1) {
			t.Errorf("row %d is %v, want Kind order", i, f.kind)
		}
		if f.name == "" || f.noun == "" || f.label == "" || len(f.exts) == 0 || f.read == nil {
			t.Errorf("%v: incomplete %+v", k, f)
		}
		for _, e := range f.exts {
			if got, ok := KindOf("x" + e); !ok || got != k {
				t.Errorf("KindOf(x%s) = %v, %v", e, got, ok)
			}
		}
		if k.CanExport() != (k.MenuTitle() != "") || k.CanExport() != (k.About() != "") {
			t.Errorf("%v: export %v, menu %q, about %q", k, k.CanExport(), k.MenuTitle(), k.About())
		}
	}
	if XLSX.MenuTitle() != "Microsoft Excel (.xlsx)" || !XLSX.HoldsSheets() || CSV.HoldsSheets() {
		t.Errorf("XLSX: %q, holds sheets %v", XLSX.MenuTitle(), XLSX.HoldsSheets())
	}
	if !SQLite.HasTables() || XLSX.HasTables() {
		t.Errorf("only SQLite has tables")
	}
	if k := Kind(99); k.String() != "unknown" || k.Ext() != "" || k.CanExport() {
		t.Errorf("unknown kind %q %q %v", k, k.Ext(), k.CanExport())
	}
	if _, ok := KindOf("notes.txt"); ok {
		t.Errorf("KindOf(.txt) recognized")
	}
}
