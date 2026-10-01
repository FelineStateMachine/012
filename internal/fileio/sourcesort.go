package fileio

import (
	"bufio"
	"cmp"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"strings"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// A Parquet view's rows are a file of row numbers in the view's order,
// 8 bytes each, which a page reads with ReadAt: a filter writes the rows
// it lets through as it streams them, and a sort is an external merge
// sort (extsort.go) of each row's number and keys. Memory stays bounded
// whatever the row count; the disk holds the runs and the order, about
// 8 bytes a row plus the keys.

// sortVal is one key of a row as a sort compares it: its kind's rank
// (numbers, text, booleans, errors, then blanks), and its number or its
// text in lower case.
type sortVal struct {
	rank uint8
	num  float64
	str  string
}

// blankRank is a blank's, last in either direction.
const blankRank = 4

func sortValOf(v sheet.Value) sortVal {
	switch v.Kind {
	case sheet.Number:
		return sortVal{rank: 0, num: v.Num}
	case sheet.Text:
		return sortVal{rank: 1, str: strings.ToLower(v.Str)}
	case sheet.Bool:
		return sortVal{rank: 2, num: v.Num}
	case sheet.Error:
		return sortVal{rank: 3, str: strings.ToLower(v.Str)}
	}
	return sortVal{rank: blankRank}
}

// sortRec is a row being sorted: its number and its keys.
type sortRec struct {
	row  int64
	keys []sortVal
}

// size is about what the record holds in memory.
func (r *sortRec) size() int {
	n := 48 + 32*len(r.keys)
	for _, k := range r.keys {
		n += len(k.str)
	}
	return n
}

// compareRecs orders rows as a sheet's sort does (sheet.compareRows):
// key by key, blanks last in either direction, Z to A reversing the
// rest, and ties in the source's order.
func compareRecs(a, b *sortRec, desc []bool) int {
	for i := range a.keys {
		x, y := &a.keys[i], &b.keys[i]
		if (x.rank == blankRank) != (y.rank == blankRank) {
			if x.rank == blankRank {
				return 1
			}
			return -1
		}
		d := cmp.Compare(x.rank, y.rank)
		if d == 0 {
			switch x.rank {
			case 0, 2:
				d = cmp.Compare(x.num, y.num)
			case 1, 3:
				d = strings.Compare(x.str, y.str)
			}
		}
		if desc[i] {
			d = -d
		}
		if d != 0 {
			return d
		}
	}
	return cmp.Compare(a.row, b.row)
}

// extSorter sorts a view's rows by their keys in bounded memory.
type extSorter struct{ *extSort[sortRec] }

func newExtSorter(dir string, desc []bool) *extSorter {
	keys := len(desc)
	return &extSorter{newExtSort(dir, recCodec[sortRec]{
		cmp:  func(a, b sortRec) int { return compareRecs(&a, &b, desc) },
		size: (*sortRec).size,
		put:  writeRec,
		get:  func(r *bufio.Reader, rec *sortRec) error { return readRec(r, rec, keys) },
	})}
}

// add takes a row's keys.
func (s *extSorter) add(row int64, keys []sheet.LiveCell) error {
	r := sortRec{row: row, keys: make([]sortVal, len(keys))}
	for i, k := range keys {
		r.keys[i] = sortValOf(k.V)
	}
	return s.extSort.add(r)
}

// finish writes the sorted rows' numbers to out and removes the runs.
func (s *extSorter) finish(out io.Writer) (n int64, err error) {
	o, err := s.sorted()
	if err != nil {
		return 0, err
	}
	defer o.close()
	for r := o.next(); r != nil; r = o.next() {
		if err := writeRow(out, r.row); err != nil {
			return n, err
		}
		n++
	}
	return n, o.err()
}

func writeRow(w io.Writer, row int64) error {
	var b [8]byte
	binary.LittleEndian.PutUint64(b[:], uint64(row))
	_, err := w.Write(b[:])
	return err
}

// writeRec writes a record to a run: its row, then each key's rank and
// its number or text.
func writeRec(w *bufio.Writer, r *sortRec) {
	var b [binary.MaxVarintLen64]byte
	w.Write(b[:binary.PutUvarint(b[:], uint64(r.row))])
	for _, k := range r.keys {
		w.WriteByte(k.rank)
		switch k.rank {
		case 0, 2:
			binary.LittleEndian.PutUint64(b[:8], math.Float64bits(k.num))
			w.Write(b[:8])
		case 1, 3:
			w.Write(b[:binary.PutUvarint(b[:], uint64(len(k.str)))])
			w.WriteString(k.str)
		}
	}
}

// readRec reads a record of keys keys from a run, io.EOF at its end.
func readRec(r *bufio.Reader, rec *sortRec, keys int) error {
	row, err := binary.ReadUvarint(r)
	if err != nil {
		return err
	}
	rec.row = int64(row)
	rec.keys = rec.keys[:0]
	for range keys {
		k, err := readKey(r)
		if err != nil {
			if errors.Is(err, io.EOF) {
				err = io.ErrUnexpectedEOF
			}
			return err
		}
		rec.keys = append(rec.keys, k)
	}
	return nil
}

func readKey(r *bufio.Reader) (sortVal, error) {
	rank, err := r.ReadByte()
	if err != nil {
		return sortVal{}, err
	}
	k := sortVal{rank: rank}
	switch rank {
	case 0, 2:
		var b [8]byte
		if _, err := io.ReadFull(r, b[:]); err != nil {
			return k, err
		}
		k.num = math.Float64frombits(binary.LittleEndian.Uint64(b[:]))
	case 1, 3:
		n, err := binary.ReadUvarint(r)
		if err != nil {
			return k, err
		}
		b := make([]byte, n)
		if _, err := io.ReadFull(r, b); err != nil {
			return k, err
		}
		k.str = string(b)
	}
	return k, nil
}

// rowFile is a view's rows as a file of row numbers in its order.
type rowFile struct {
	f *os.File
	n int64
}

// newRowFile makes the file rows are written to, in dir.
func newRowFile(dir string) (*os.File, error) { return os.CreateTemp(dir, "012-view-*.rows") }

// read returns the row numbers at positions from to from+n, as many as
// there are.
func (rf *rowFile) read(from int64, n int) ([]int64, error) {
	if from < 0 || from >= rf.n || n <= 0 {
		return nil, nil
	}
	n = int(min(int64(n), rf.n-from))
	b := make([]byte, 8*n)
	if _, err := rf.f.ReadAt(b, 8*from); err != nil {
		return nil, err
	}
	out := make([]int64, n)
	for i := range out {
		out[i] = int64(binary.LittleEndian.Uint64(b[8*i:]))
	}
	return out, nil
}

// close closes and removes the file.
func (rf *rowFile) close() error {
	name := rf.f.Name()
	return errors.Join(rf.f.Close(), os.Remove(name))
}
