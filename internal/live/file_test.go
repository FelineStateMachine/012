package live

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/fileio"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// rowsText writes an update's rows as a test reads them.
func rowsText(rows []sheet.LiveRow) string {
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

// follow starts following a file in a temporary directory, returning it
// and a function writing it.
func follow(t *testing.T, name string) (*File, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	k, ok := KindOf(name, "")
	if !ok {
		t.Fatalf("no kind for %s", name)
	}
	f := NewFile(name, path, k, fileio.Options{})
	t.Cleanup(f.Close)
	return f, path
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(text); err != nil {
		t.Fatal(err)
	}
}

func poll(t *testing.T, f *File) (Update, bool) {
	t.Helper()
	return f.Poll(context.Background())
}

func TestFileAppendAndPartialLine(t *testing.T) {
	f, path := follow(t, "log.csv")
	u, ok := poll(t, f)
	if !ok || u.Err != "the file isn't there" {
		t.Fatalf("missing file: %+v", u)
	}
	if _, ok := poll(t, f); ok {
		t.Fatal("the same error twice")
	}
	write(t, path, "when,n\n1,10\n2,2")
	u, ok = poll(t, f)
	if !ok || !u.Reset || u.Err != "" || rowsText([]sheet.LiveRow{u.Header}) != "when,n" || rowsText(u.Rows) != "1,10" {
		t.Fatalf("first read: %+v", u)
	}
	if _, ok := poll(t, f); ok {
		t.Fatal("nothing changed, yet an update")
	}
	appendTo(t, path, "0\n3,30\n")
	u, ok = poll(t, f)
	if !ok || u.Reset || rowsText(u.Rows) != "2,20|3,30" {
		t.Fatalf("append: %+v", u)
	}
}

func TestFileTruncateRotateRewrite(t *testing.T) {
	f, path := follow(t, "log.tsv")
	write(t, path, "a\n1\n2\n")
	poll(t, f)
	// Truncated: read again from the start.
	write(t, path, "a\n9\n")
	if u, ok := poll(t, f); !ok || !u.Reset || rowsText(u.Rows) != "9" {
		t.Fatalf("truncated: %+v", u)
	}
	// Rewritten in place, longer: the bytes read before differ.
	write(t, path, "b\n7\n8\n")
	if u, ok := poll(t, f); !ok || !u.Reset || rowsText(u.Rows) != "7|8" || u.Header[0].V.Str != "b" {
		t.Fatalf("rewritten: %+v", u)
	}
	// Rotated: renamed away, and a new file in its place.
	os.Rename(path, path+".1")
	write(t, path, "c\n5\n6\n7\n")
	if u, ok := poll(t, f); !ok || !u.Reset || rowsText(u.Rows) != "5|6|7" {
		t.Fatalf("rotated: %+v", u)
	}
	appendTo(t, path+".1", "gone\n") // the old file no longer counts
	if u, ok := poll(t, f); ok {
		t.Fatalf("the rotated file was read: %+v", u)
	}
}

func TestFileMaxRead(t *testing.T) {
	f, path := follow(t, "big.csv")
	f.MaxRead = 8
	write(t, path, "a\n1\n2\n3\n4\n5\n6\n")
	var all []sheet.LiveRow
	for i := 0; ; i++ {
		u, _ := poll(t, f)
		all = append(all, u.Rows...)
		if !u.More {
			break
		}
		if i > 10 {
			t.Fatal("never done")
		}
	}
	if rowsText(all) != "1|2|3|4|5|6" {
		t.Fatalf("%q", rowsText(all))
	}
}

func TestFileNDJSON(t *testing.T) {
	f, path := follow(t, "events.ndjson")
	if f.kind != fileio.JSON {
		t.Skip("ndjson isn't JSON's extension here")
	}
	write(t, path, `{"a": 1}`+"\n"+`{"a": 2, "b"`)
	u, _ := poll(t, f)
	if rowsText(u.Rows) != "1" {
		t.Fatalf("%+v", u)
	}
	appendTo(t, path, `: "x"}`+"\n")
	u, _ = poll(t, f)
	if rowsText(u.Rows) != "2,x" || rowsText([]sheet.LiveRow{u.Header}) != "a,b" {
		t.Fatalf("%+v", u)
	}
}

func TestFileRewrittenWholeDebounced(t *testing.T) {
	f, path := follow(t, "book.xlsx")
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	f.Now = func() time.Time { return now }
	save := func(v string) {
		s := sheet.New()
		s.Set(sheet.Addr{}, "n")
		s.Set(sheet.Addr{Row: 1}, v)
		if _, err := fileio.Export(context.Background(), path, fileio.XLSX, fileio.SnapBook(s), fileio.ExportOptions{}); err != nil {
			t.Fatal(err)
		}
	}
	save("1")
	u, ok := poll(t, f)
	if !ok || !u.Reset || rowsText(u.Rows) != "1" {
		t.Fatalf("first read: %+v", u)
	}
	// Rewritten twice in quick succession: read once, after it settles.
	save("22")
	if _, ok := poll(t, f); ok {
		t.Fatal("read while changing")
	}
	now = now.Add(100 * time.Millisecond)
	save("333")
	os.Chtimes(path, now, now)
	if _, ok := poll(t, f); ok {
		t.Fatal("read while still changing")
	}
	now = now.Add(100 * time.Millisecond)
	if _, ok := poll(t, f); ok {
		t.Fatal("read before the debounce")
	}
	now = now.Add(time.Second)
	if u, ok := poll(t, f); !ok || rowsText(u.Rows) != "333" {
		t.Fatalf("after settling: %+v %v", u, ok)
	}
	if _, ok := poll(t, f); ok {
		t.Fatal("read again, unchanged")
	}
}

func TestKindOf(t *testing.T) {
	if k, ok := KindOf("x.txt", "tsv"); !ok || k != fileio.TSV {
		t.Fatal("format by name")
	}
	if k, ok := KindOf("x.nuon", ""); !ok || k != fileio.NUON {
		t.Fatal("format by extension")
	}
	if _, ok := KindOf("x.txt", ""); ok {
		t.Fatal("an unknown format")
	}
}

func TestUpdateOp(t *testing.T) {
	at := time.Now()
	op := Update{Reset: true, Err: "e", Note: "n", At: at}.Op(7)
	if op.Link != 7 || !op.Reset || op.Err != "e" || op.Note != "n" || !op.At.Equal(at) {
		t.Fatalf("%+v", op)
	}
}
