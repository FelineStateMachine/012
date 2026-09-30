package stress

import (
	"database/sql"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"

	"github.com/parquet-go/parquet-go"
	_ "modernc.org/sqlite" // registers the "sqlite" driver
)

// Sources for the benchmarks of linked sources: a Parquet file and a
// SQLite table of sales, rows rows each (ten million for make stress),
// written once into $STRESS_DIR/generated, or a directory the caller
// gives, and kept between runs, as generating them takes a while.

// SaleRow is a row of the generated sources: an id, one of eight
// categories, an amount and a date. Its columns are A to D of the
// sources' tabs.
type SaleRow struct {
	ID     int64   `parquet:"id"`
	Cat    string  `parquet:"cat,dict"`
	Amount float64 `parquet:"amount"`
	Day    int32   `parquet:"day,date"`
}

// SaleCats are the categories, in the order rows draw them from.
var SaleCats = []string{"north", "south", "east", "west", "central", "coast", "hills", "islands"}

// Sale is row i of the sources, the same whenever it's asked for.
func Sale(i int) SaleRow {
	r := rand.New(rand.NewPCG(uint64(i), 12))
	return SaleRow{ID: int64(i), Cat: SaleCats[r.IntN(len(SaleCats))], Amount: float64(r.IntN(1_000_000)) / 100,
		Day: int32(time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC).Unix()/86400) + int32(r.IntN(2000))}
}

// GeneratedDir is where generated sources are kept: $STRESS_DIR's
// generated folder, or fallback.
func GeneratedDir(fallback string) string {
	if d := os.Getenv("STRESS_DIR"); d != "" {
		dir := filepath.Join(d, "generated")
		if err := os.MkdirAll(dir, 0o755); err == nil {
			return dir
		}
	}
	return fallback
}

// SalesParquet is the Parquet source of rows rows in dir, written unless
// it's there.
func SalesParquet(dir string, rows int) (string, error) {
	name := filepath.Join(dir, fmt.Sprintf("sales-%d.parquet", rows))
	if _, err := os.Stat(name); err == nil {
		return name, nil
	}
	tmp := name + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	defer f.Close()
	w := parquet.NewGenericWriter[SaleRow](f, parquet.MaxRowsPerRowGroup(1<<20))
	batch := make([]SaleRow, 1<<16)
	for i := 0; i < rows; i += len(batch) {
		n := min(len(batch), rows-i)
		for j := range n {
			batch[j] = Sale(i + j)
		}
		if _, err := w.Write(batch[:n]); err != nil {
			return "", err
		}
	}
	if err := w.Close(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return name, os.Rename(tmp, name)
}

// SalesSQLite is the SQLite source of rows rows in dir, a table named
// sales, written unless it's there.
func SalesSQLite(dir string, rows int) (string, error) {
	name := filepath.Join(dir, fmt.Sprintf("sales-%d.sqlite", rows))
	if _, err := os.Stat(name); err == nil {
		return name, nil
	}
	tmp := name + ".part"
	os.Remove(tmp)
	db, err := sql.Open("sqlite", tmp+"?_pragma=journal_mode(OFF)&_pragma=synchronous(OFF)")
	if err != nil {
		return "", err
	}
	defer db.Close()
	tx, err := db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("CREATE TABLE sales (id INTEGER, cat TEXT, amount REAL, day DATE)"); err != nil {
		return "", err
	}
	ins, err := tx.Prepare("INSERT INTO sales VALUES (?, ?, ?, ?)")
	if err != nil {
		return "", err
	}
	for i := range rows {
		r := Sale(i)
		day := time.Unix(int64(r.Day)*86400, 0).UTC().Format(time.DateOnly)
		if _, err := ins.Exec(r.ID, r.Cat, r.Amount, day); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	if err := db.Close(); err != nil {
		return "", err
	}
	return name, os.Rename(tmp, name)
}
