package fileio

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"

	"github.com/FelineStateMachine/012/internal/sheet"
)

type trip struct {
	City    string    `parquet:"city"`
	Miles   float64   `parquet:"miles"`
	Riders  int32     `parquet:"riders"`
	Paid    bool      `parquet:"paid"`
	Day     int32     `parquet:"day,date"` // days since 1970
	Started time.Time `parquet:"started,timestamp(millisecond)"`
	Note    *string   `parquet:"note,optional"`
	Tags    []string  `parquet:"tags,list"`
}

func TestParquetImport(t *testing.T) {
	note := "late"
	day := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	rows := []trip{
		{City: "Boston", Miles: 12.5, Riders: 2, Paid: true, Day: int32(day.Unix() / 86400), Started: day.Add(14*time.Hour + 30*time.Minute), Note: &note, Tags: []string{"a", "b"}},
		{City: "=evil()", Miles: 3, Riders: 1, Day: int32(day.Unix()/86400) + 1, Started: day},
	}
	name := filepath.Join(t.TempDir(), "trips.parquet")
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	w := parquet.NewGenericWriter[trip](f)
	if _, err := w.Write(rows); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	prog := NewProgress()
	res, err := Import(context.Background(), name, Options{Progress: prog})
	if err != nil {
		t.Fatal(err)
	}
	s := res.Sheet
	for cell, want := range map[string]string{
		"A1": "city", "B1": "miles", "E1": "day", "G1": "note", "H1": "tags.list.element",
		"A2": "Boston", "B2": "12.5", "C2": "2", "D2": "TRUE", "E2": "9/26/2026", "F2": "9/26/2026 14:30:00",
		"G2": "late", "H2": "a, b",
		"A3": "=evil()", "D3": "FALSE", "E3": "9/27/2026", "G3": "", "H3": "",
	} {
		if g := shown(s, addr(t, cell)); g != want {
			t.Errorf("%s shows %q, want %q", cell, g, want)
		}
	}
	if rows, frac := prog.Get(); rows != 3 || frac != 1 {
		t.Errorf("progress %d, %v", rows, frac)
	}
	if len(res.Notes) != 1 || res.Notes[0] != "repeated values joined with commas" {
		t.Errorf("notes %q", res.Notes)
	}
}

// TestParquetPastLimit reads rows up to the sheet's last and counts the
// rest from the file's metadata.
func TestParquetPastLimit(t *testing.T) {
	type num struct {
		N int32 `parquet:"n"`
	}
	rows := make([]num, sheet.MaxRows+4)
	for i := range rows {
		rows[i].N = int32(i + 1)
	}
	name := filepath.Join(t.TempDir(), "nums.parquet")
	f, err := os.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	w := parquet.NewGenericWriter[num](f, parquet.MaxRowsPerRowGroup(1000))
	if _, err := w.Write(rows); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f.Close()

	res, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	last := sheet.Addr{Row: sheet.MaxRows - 1}
	if g := shown(res.Sheet, last); g != "8191" {
		t.Errorf("last row shows %q, want 8191", g)
	}
	if res.Rows != len(rows)+1 {
		t.Errorf("rows %d, want %d", res.Rows, len(rows)+1)
	}
	if len(res.Notes) != 1 || res.Notes[0] != "only the first 8,192 rows fit; 5 rows left out" {
		t.Errorf("notes %q", res.Notes)
	}
}

func TestParquetNotParquet(t *testing.T) {
	name := filepath.Join(t.TempDir(), "x.parquet")
	os.WriteFile(name, []byte("not parquet"), 0o644)
	if _, err := Import(context.Background(), name, Options{}); err == nil {
		t.Errorf("imported junk")
	}
}
