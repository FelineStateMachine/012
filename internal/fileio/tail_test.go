package fileio

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// liveText writes rows as a test reads them: cells' values by commas, rows
// by bars.
func liveText(rows ...sheet.LiveRow) string {
	var out []string
	for _, r := range rows {
		var cells []string
		for _, c := range r {
			cells = append(cells, c.V.String())
		}
		out = append(out, strings.Join(cells, ","))
	}
	return strings.Join(out, "|")
}

// feed feeds pieces to a tail, returning what each completed.
func feed(t *testing.T, tl *Tail, pieces ...string) []TailRows {
	t.Helper()
	var out []TailRows
	for _, p := range pieces {
		got, err := tl.Feed([]byte(p))
		if err != nil {
			t.Fatalf("feeding %q: %v", p, err)
		}
		out = append(out, got)
	}
	return out
}

func TestTailCSV(t *testing.T) {
	head := "when,level,msg\n9/29/2026,3,\"two\nlines\"\n"
	tl, err := NewTail(CSV, []byte(head), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	got := feed(t, tl, "when,level,m", "sg\n9/29/2026,3,\"two\n", "lines\"\n10/1/2026,4,partial", "\n")
	if liveText(got[0].Header) != "" || len(got[0].Rows) != 0 {
		t.Fatalf("a partial header was read: %+v", got[0])
	}
	if liveText(got[1].Header) != "when,level,msg" || len(got[1].Rows) != 0 {
		t.Fatalf("header: %+v", got[1])
	}
	if len(got[2].Rows) != 1 || got[2].Rows[0][2].V.Str != "two lines" && got[2].Rows[0][2].V.Str != "two\nlines" {
		t.Fatalf("a quoted field across pieces: %+v", got[2].Rows)
	}
	if got[2].Rows[0][0].F.IsZero() || got[2].Rows[0][1].V.Num != 3 {
		t.Fatalf("fields weren't typed: %+v", got[2].Rows[0])
	}
	if len(got[3].Rows) != 1 || got[3].Rows[0][2].V.Str != "partial" {
		t.Fatalf("the last line: %+v", got[3])
	}
}

func TestTailEncodingsAndDelimiters(t *testing.T) {
	// A UTF-16 file split mid-character, with a byte order mark.
	text := "a\tb\n1\tç\n"
	var utf16 []byte
	utf16 = append(utf16, 0xFF, 0xFE)
	for _, r := range text {
		utf16 = append(utf16, byte(r), byte(r>>8))
	}
	tl, err := NewTail(TSV, utf16, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	var rows []sheet.LiveRow
	var header sheet.LiveRow
	for i := 0; i < len(utf16); i += 3 {
		got, err := tl.Feed(utf16[i:min(i+3, len(utf16))])
		if err != nil {
			t.Fatal(err)
		}
		if got.Header != nil {
			header = got.Header
		}
		rows = append(rows, got.Rows...)
	}
	if liveText(header) != "a,b" || liveText(rows...) != "1,ç" {
		t.Fatalf("UTF-16: %q %q", liveText(header), liveText(rows...))
	}
	// Semicolons sniffed, in a locale with decimal commas.
	head := "x;y\n1,5;2\n"
	tl, _ = NewTail(CSV, []byte(head), deLocale(t))
	defer tl.Close()
	got := feed(t, tl, head)
	if len(got[0].Rows) != 1 || got[0].Rows[0][0].V.Num != 1.5 {
		t.Fatalf("semicolons: %+v", got[0])
	}
}

func TestTailNDJSONAndNUON(t *testing.T) {
	tl, err := NewTail(JSON, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tl.Close()
	got := feed(t, tl, `{"a": 1, "b": "x"}`+"\n"+`{"a": 2`, `, "c": true}`+"\n")
	if liveText(got[0].Header) != "a,b" || liveText(got[0].Rows...) != "1,x" {
		t.Fatalf("first piece: %q %q", liveText(got[0].Header), liveText(got[0].Rows...))
	}
	if liveText(got[1].Header) != "a,b,c" || liveText(got[1].Rows...) != "2,,TRUE" {
		t.Fatalf("a new column: %q %q", liveText(got[1].Header), liveText(got[1].Rows...))
	}
	// NUON's table form, streaming, with types.
	tl, _ = NewTail(NUON, nil, nil)
	defer tl.Close()
	got = feed(t, tl, "[[size, took]; [1kb, 2sec],\n", "[3b, 1min]]\n")
	if liveText(got[0].Header) != "size,took" || len(got[0].Rows) != 1 || got[0].Rows[0][0].V.Num != 1000 || got[0].Rows[0][0].F.IsZero() {
		t.Fatalf("table form: %+v", got[0])
	}
	if len(got[1].Rows) != 1 || got[1].Header != nil {
		t.Fatalf("second piece: %+v", got[1])
	}
	// A syntax error ends the tail.
	tl, _ = NewTail(NUON, nil, nil)
	defer tl.Close()
	if _, err := tl.Feed([]byte("{a: }}\n")); err == nil {
		t.Fatal("no error for bad NUON")
	}
	if _, err := tl.Feed([]byte("{a: 1}\n")); err == nil {
		t.Fatal("a failed tail read on")
	}
}

func TestTailClose(t *testing.T) {
	tl, _ := NewTail(CSV, nil, nil)
	feed(t, tl, "a,b\n1,")
	tl.Close()
	select {
	case <-tl.p.done:
	case <-time.After(5 * time.Second):
		t.Fatal("the tail's goroutine didn't end")
	}
	if _, err := tl.Feed([]byte("2\n")); err == nil {
		t.Fatal("a closed tail read on")
	}
	if _, err := NewTail(XLSX, nil, nil); err == nil {
		t.Fatal("a tail of XLSX")
	}
}

func deLocale(t *testing.T) *locale.Locale {
	t.Helper()
	de, ok := locale.Lookup("de-DE")
	if !ok {
		t.Fatal("no de-DE")
	}
	return de
}

func TestRowsOfImport(t *testing.T) {
	name := filepath.Join(t.TempDir(), "t.csv")
	os.WriteFile(name, []byte("a,b\n1,$2\n,x\n"), 0o644)
	res, err := Import(context.Background(), name, Options{})
	if err != nil {
		t.Fatal(err)
	}
	got := Rows(res.Sheet)
	if liveText(got.Header) != "a,b" || liveText(got.Rows...) != "1,2|,x" || got.Rows[0][1].F.IsZero() {
		t.Fatalf("%q %q", liveText(got.Header), liveText(got.Rows...))
	}
	if got := Rows(sheet.New()); got.Header != nil || got.Rows != nil {
		t.Fatalf("an empty sheet: %+v", got)
	}
}
