package fileio

import (
	"context"
	"database/sql"
	"encoding/binary"
	"path/filepath"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// TestSQLiteDeclaredFormats checks the formats a SQLite source's
// columns show in, from their declared types.
func TestSQLiteDeclaredFormats(t *testing.T) {
	name := filepath.Join(t.TempDir(), "typed.sqlite")
	db, err := sql.Open("sqlite", name)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE t (n INTEGER, price DECIMAL(10,2), rate NUMERIC(8, 4), plain NUMERIC, paid MONEY,
		day DATE, at DATETIME, stamp TIMESTAMP, clock TIME, note TEXT)`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	src, err := OpenSource(context.Background(), SourceSpec{Path: name, TempDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	defer src.Close()
	want := []SourceColumn{
		{Name: "n", Numeric: true},
		{Name: "price", Format: sheet.Format{Kind: sheet.FmtNumber, Decimals: 2}, Numeric: true},
		{Name: "rate", Format: sheet.Format{Kind: sheet.FmtNumber, Decimals: 4}, Numeric: true},
		{Name: "plain", Numeric: true},
		{Name: "paid", Format: sheet.Format{Kind: sheet.FmtCurrency, Decimals: 2}, Numeric: true},
		{Name: "day", Format: sheet.Format{Kind: sheet.FmtDate}, Numeric: true},
		{Name: "at", Format: sheet.Format{Kind: sheet.FmtDateTime}, Numeric: true},
		{Name: "stamp", Format: sheet.Format{Kind: sheet.FmtDateTime}, Numeric: true},
		{Name: "clock", Format: sheet.Format{Kind: sheet.FmtTime}, Numeric: true},
		{Name: "note"},
	}
	got := src.Columns()
	if len(got) != len(want) {
		t.Fatalf("columns %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column %d: %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestParquetInterval checks an INTERVAL's 12 bytes read as days: months
// of 30 days, days, and milliseconds.
func TestParquetInterval(t *testing.T) {
	b := make([]byte, 12)
	binary.LittleEndian.PutUint32(b, 1)
	binary.LittleEndian.PutUint32(b[4:], 2)
	binary.LittleEndian.PutUint32(b[8:], 43_200_000)
	d, ok := interval(parquet.FixedLenByteArrayValue(b))
	if !ok || d != 32.5 {
		t.Errorf("interval %v %v, want 32.5 days", d, ok)
	}
	if _, ok := interval(parquet.ByteArrayValue([]byte{1, 2})); ok {
		t.Error("read 2 bytes as an interval")
	}
	c := parquetColumn{kind: parquet.FixedLenByteArray, interval: true}
	if lc, ok := parquetValue(parquet.FixedLenByteArrayValue(b), c); !ok || lc.F.Kind != sheet.FmtDuration || lc.V.Num != 32.5 {
		t.Errorf("an interval's cell: %+v", lc)
	}
	if f, num := parquetFormat(c); f.Kind != sheet.FmtDuration || !num {
		t.Errorf("an interval column shows in %+v", f)
	}
}
