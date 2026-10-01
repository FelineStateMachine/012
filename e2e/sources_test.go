package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/parquet-go/parquet-go"
)

// trip is a row of the Parquet file the linked sources' tests read: a
// ride, how it was paid, its fare, its miles and its day.
type trip struct {
	Trip    int64   `parquet:"trip"`
	Payment string  `parquet:"payment,dict"`
	Fare    int64   `parquet:"fare,decimal(2:12)"`
	Miles   float64 `parquet:"miles"`
	Day     int32   `parquet:"day,date"`
}

// payments are the ways trips are paid, in the order rows take them.
var payments = []string{"card", "cash", "card", "app", "card", "cash", "voucher"}

// tripAt is trip i, the same whenever it's asked for.
func tripAt(i int) trip {
	day := int32(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC).Unix()/86400) + int32(i%270)
	return trip{Trip: int64(i + 1), Payment: payments[i%len(payments)],
		Fare: int64(i*7919%9000 + 300), Miles: float64((i*104729)%400) / 10, Day: day}
}

// writeTrips writes rows trips to trips.parquet in dir.
func writeTrips(t *testing.T, dir string, rows int) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, "trips.parquet"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	w := parquet.NewGenericWriter[trip](f, parquet.MaxRowsPerRowGroup(1<<18))
	batch := make([]trip, 1<<14)
	for i := 0; i < rows; i += len(batch) {
		n := min(len(batch), rows-i)
		for j := range n {
			batch[j] = tripAt(i + j)
		}
		if _, err := w.Write(batch[:n]); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

// tripRows is how many rows the screens' source has: past the grid's
// last row.
const tripRows = 2_000_000

// linkSource links name as a source through the palette, and waits for
// its rows.
func (s *session) linkSource(name string) {
	s.t.Helper()
	s.keys("<ctrl+k>", "Link a source")
	s.waitFor("Link a source")
	s.keys("<enter>")
	s.waitFor("Type to filter, or a path")
	s.keys(name, "<enter>")
	s.waitFor("Linked " + name)
	s.keys("<down>", "<up>") // the note gives way to the source's line
	s.waitFor("▦ " + name)
}

func TestLinkedSource(t *testing.T) {
	dir := t.TempDir()
	writeTrips(t, dir, tripRows)
	s := start(t, dir)
	s.linkSource("trips.parquet")
	s.waitFor("2,000,000 rows, 5 columns")
	s.waitFor("card")

	// The last row, numbered past the grid's last row.
	s.keys("<ctrl+down>")
	s.waitFor("2000001 ")

	// Sorted by fare, Z to A, in the background.
	s.keys("<ctrl+home>", "<right>", "<right>")
	s.keys("<ctrl+k>", "Sort sheet Z to A", "<enter>")
	s.waitFor("sorted by fare Z to A")
	s.eventually("the top fare", func() bool { return strings.Contains(s.line(1), "92.99") })

	// A formula on another sheet reads the whole source.
	s.keys("<ctrl+pgup>")
	s.waitFor("Sheet1")
	s.keys("=COUNTIF(trips[payment],\"voucher\")", "<enter>")
	s.waitFor("285,714") // tripRows / 7, a count grouped

	// Saved, the workbook keeps the link, and reads the source again.
	s.keys("<ctrl+s>")
	s.waitFor("Save as:")
	s.keys("rides", "<enter>")
	s.waitFor("rides.012")
	data, err := os.ReadFile(filepath.Join(dir, "rides.012"))
	if err != nil || !strings.Contains(string(data), `"paged":true`) || !strings.Contains(string(data), `"version": 7`) {
		t.Fatalf("the file: %v\n%s", err, data)
	}
	s.keys("<ctrl+q>")
	s.waitExit()
	s = start(t, dir, "rides.012")
	s.waitFor("285,714") // tripRows / 7, a count grouped
}
