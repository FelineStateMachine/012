// Command gensales writes the sales the linked sources demo reads
// (sources.tape): ten million rows of Parquet, an id, one of eight
// categories, an amount and a date each, the same every time
// (internal/stress's SalesParquet), into the file named on its command
// line.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/FelineStateMachine/012/internal/stress"
)

func main() {
	rows := flag.Int("rows", 10_000_000, "how many rows to write")
	flag.Parse()
	if flag.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: gensales [-rows n] out.parquet")
		os.Exit(2)
	}
	out := flag.Arg(0)
	dir, err := os.MkdirTemp(filepath.Dir(out), "gensales")
	if err == nil {
		defer os.RemoveAll(dir)
		var name string
		if name, err = stress.SalesParquet(dir, *rows); err == nil {
			err = os.Rename(name, out)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "gensales:", err)
		os.Exit(1)
	}
}
