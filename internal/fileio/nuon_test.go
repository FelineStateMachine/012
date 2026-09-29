package fileio

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/nuon"
	"github.com/FelineStateMachine/012/internal/sheet"
)

// inZone shows dates in loc for the test.
func inZone(t *testing.T, loc *time.Location) {
	t.Helper()
	old := zone
	zone = func() *time.Location { return loc }
	t.Cleanup(func() { zone = old })
}

var mountain = time.FixedZone("MDT", -6*3600)

func readNUON(t *testing.T, text string) *Result {
	t.Helper()
	res, err := ImportReader(context.Background(), "stdin", strings.NewReader(text), Options{})
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return res
}

func TestNUONCells(t *testing.T) {
	inZone(t, mountain)
	res := readNUON(t, `[[name, size, modified, took, ok, n, x, tags, meta, raw];
		["CLAUDE.md", 1646b, 2026-09-27T17:27:31+00:00, 90sec, true, 1.5, "12", [a, b], {k: 1}, 0x[DEAD]],
		[go.mod, 2mb, 2026-01-02T03:04:05.250-06:00, 1500ms, false, -3, null, [], {}, 0x[]]]`)
	s := res.Sheet
	if res.Kind != NUON || s.Name() != "stdin" || res.Rows != 3 {
		t.Errorf("kind %v, sheet %q, rows %d", res.Kind, s.Name(), res.Rows)
	}
	for a, want := range map[string]string{
		"A1": "name", "B1": "size", "J1": "raw",
		"A2": "CLAUDE.md", "B2": "1.6 kB", "C2": "9/27/2026 11:27:31", "D2": "0:01:30", "E2": "TRUE", "F2": "1.5",
		"G2": "12", "H2": "[a, b]", "I2": "{k: 1}", "J2": "0x[DEAD]",
		"B3": "2.0 MB", "C3": "1/2/2026 3:04:05", "D3": "0:00:01.500", "E3": "FALSE", "F3": "-3", "G3": "", "H3": "[]",
	} {
		if got := shown(s, addr(t, a)); got != want {
			t.Errorf("%s shows %q, want %q", a, got, want)
		}
	}
	if v := s.Value(addr(t, "G2")); v.Kind != sheet.Text {
		t.Errorf("the string 12 became %v", v)
	}
	if f := s.CellFormat(addr(t, "B2")); f.Kind != sheet.FmtSize {
		t.Errorf("size format %+v", f)
	}
}

// TestNUONColumnsFromRecords names columns as records bring them.
func TestNUONColumnsFromRecords(t *testing.T) {
	s := readNUON(t, `[{a: 1}, {b: 2, a: 3}] {c: x}`).Sheet
	for a, want := range map[string]string{"A1": "a", "B1": "b", "C1": "c", "A2": "1", "A3": "3", "B3": "2", "C4": "x"} {
		if got := shown(s, addr(t, a)); got != want {
			t.Errorf("%s shows %q, want %q", a, got, want)
		}
	}
}

func TestImportReaderSniffs(t *testing.T) {
	for text, want := range map[string]Kind{
		`[{"a": 1}]`:       JSON,
		"\n  {\"a\": 1}\n": JSON,
		`[{a: 1}]`:         NUON,
		`[[a]; [1]]`:       NUON,
		"a,b\n1,2\n":       CSV,
		"a\tb\n1\t2\n":     TSV,
		"\xEF\xBB\xBF[1]":  JSON,
		"":                 CSV,
		"name;qty\nx;1\n":  CSV,
	} {
		if res := readNUON(t, text); res.Kind != want {
			t.Errorf("%q read as %v, want %v", text, res.Kind, want)
		}
	}
	if res := readNUON(t, "a\tb\n1\t2\n"); len(res.Notes) != 0 {
		t.Errorf("TSV notes %q", res.Notes)
	}
	if _, err := ImportReader(context.Background(), "stdin", strings.NewReader("[1, 2"), Options{}); err == nil {
		t.Error("an unterminated list read")
	}
}

func TestNUONMaxCells(t *testing.T) {
	var b strings.Builder
	b.WriteString("[[a, b];")
	for i := range 10 {
		b.WriteString(" [" + string(rune('0'+i)) + ", x]")
	}
	b.WriteString("]")
	res, err := ImportReader(context.Background(), "stdin", strings.NewReader(b.String()), Options{MaxCells: 9})
	if err != nil {
		t.Fatal(err)
	}
	if n := res.Sheet.Len(); n != 8 {
		t.Errorf("%d cells kept, want the header and 3 rows", n)
	}
	if len(res.Notes) != 1 || !strings.Contains(res.Notes[0], "only the first 4 rows fit in max-cells (9 cells); 7 rows left out") {
		t.Errorf("notes %q", res.Notes)
	}
}

// encode writes the whole sheet as k.
func encode(t *testing.T, s *sheet.Sheet, k Kind) string {
	t.Helper()
	var b bytes.Buffer
	if _, err := Encode(&b, k, Snap(s, sheet.Rect{}, s.Name())); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

func TestEncodeKeepsTypes(t *testing.T) {
	inZone(t, mountain)
	s := build(t, map[string]string{
		"A1": "name", "B1": "when", "C1": "n", "D1": "ok", "E1": "", "F1": "t",
		"A2": "a b", "B2": "9/27/2026 11:27:31", "C2": "1.5", "D2": "TRUE", "E2": "=1/0", "F2": "3:00 PM",
		"A3": "'12", "B3": "9/27/2026", "C3": "42", "F3": "[1, 2]",
	})
	s.SetFormat(sheet.Rect{From: addr(t, "C3"), To: addr(t, "C3")}, sheet.Preset(sheet.FmtSize))
	want := `[[name, when, n, ok, E, t]; ["a b", 2026-09-27T11:27:31-06:00, 1.5, true, "#DIV/0!", 54000000000000ns],
["12", 2026-09-27T00:00:00-06:00, 42b, null, null, [1, 2]]]
`
	if got := encode(t, s, NUON); got != want {
		t.Errorf("NUON:\n%s\nwant\n%s", got, want)
	}
	wantJSON := `[
{"name":"a b","when":"2026-09-27T11:27:31-06:00","n":1.5,"ok":true,"E":"#DIV/0!","t":54000000000000},
{"name":"12","when":"2026-09-27T00:00:00-06:00","n":42,"ok":null,"E":null,"t":[1,2]}
]
`
	if got := encode(t, s, JSON); got != wantJSON {
		t.Errorf("JSON:\n%s\nwant\n%s", got, wantJSON)
	}
	if got := encode(t, sheet.New(), NUON); got != "[]\n" {
		t.Errorf("empty sheet %q", got)
	}
	if got := encode(t, s, CSV); !strings.HasPrefix(got, "name,when,n,ok,,t\n") {
		t.Errorf("CSV %q", got)
	}
	if _, err := Encode(&bytes.Buffer{}, XLSX, Snap(s, sheet.Rect{}, "x")); err == nil {
		t.Error("XLSX encoded to a stream")
	}
}

// Text formats send the rows a filter shows, as Sheets copies a filtered
// range.
func TestEncodeFiltered(t *testing.T) {
	s := build(t, map[string]string{"A1": "name", "B1": "n", "A2": "a", "B2": "1", "A3": "b", "B3": "2", "A4": "c", "B4": "3"})
	s.CreateFilter(sheet.Rect{To: addr(t, "B4")})
	s.FilterColumn(0, sheet.Criteria{Hidden: []string{"b"}})
	if got := encode(t, s, NUON); got != "[[name, n]; [a, 1],\n[c, 3]]\n" {
		t.Errorf("NUON %q", got)
	}
	if got := encode(t, s, CSV); got != "name,n\na,1\nc,3\n" {
		t.Errorf("CSV %q", got)
	}
	if got := encode(t, s, JSON); strings.Contains(got, `"b"`) {
		t.Errorf("JSON %q", got)
	}
}

// TestNUONRoundTrip reads a table and writes it back as the same NUON.
func TestNUONRoundTrip(t *testing.T) {
	inZone(t, mountain)
	for _, text := range []string{
		`[[name, size, modified, took, ok, n, tags, meta, raw]; ["CLAUDE.md", 1646b, 2026-09-27T11:27:31.351890-06:00, 90000000000ns, true, 1.5, [a, b], {k: 1}, 0x[DEAD]], [x, 0b, 2026-01-02T03:04:05-06:00, 1500000000ns, false, -3, [], {}, 0x[]]]`,
		`[[a, b]; [1, null], ["x y", "true"]]`, // an empty string would come back null: a blank cell
	} {
		s := readNUON(t, text).Sheet
		got, want := parse(t, encode(t, s, NUON)), parse(t, text)
		if !nuon.Equal(got, want) {
			t.Errorf("read\n%s\nwrote\n%s", want, got)
		}
	}
}

func parse(t *testing.T, text string) nuon.Value {
	t.Helper()
	v, err := nuon.Parse([]byte(text))
	if err != nil {
		t.Fatalf("%s: %v", text, err)
	}
	return v
}

// TestNUONThroughNu sends what nushell writes through a sheet and back
// to nushell, which reads the same table, when nu is installed.
func TestNUONThroughNu(t *testing.T) {
	nu, err := exec.LookPath("nu")
	if err != nil {
		t.Skip("nu isn't installed")
	}
	inZone(t, time.Local)
	run := func(script, in string) string {
		cmd := exec.Command(nu, "--no-config-file", "--stdin", "-c", script)
		cmd.Stdin = strings.NewReader(in)
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("nu -c %q: %v", script, err)
		}
		return strings.TrimSpace(string(out))
	}
	table := `[[name, size, modified, took, ok, n]; [a, 1646b, 2026-09-27T11:27:31.351890163-06:00, 90sec, true, 1.5], [b, 2mb, 2026-01-02, 1.5sec, false, -3]]`
	// Dates come back in the local zone, to the microsecond.
	norm := ` | update modified {|r| $r.modified | date to-timezone local | format date "%+" }`
	theirs := run(table+" | to nuon", "")
	s := readNUON(t, theirs).Sheet
	if got, want := run("from nuon"+norm+" | to nuon", encode(t, s, NUON)), run(table+norm+" | to nuon", ""); got != strings.ReplaceAll(want, ".351890163", ".351890") {
		t.Errorf("nu reads\n%s\nwant\n%s", got, want)
	}
	if got := run("from json | length", encode(t, s, JSON)); got != "2" {
		t.Errorf("nu reads %s JSON rows", got)
	}
}

func TestNUONFiles(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "files.nuon")
	os.WriteFile(path, []byte(`[[name, size]; [a, 1kb]]`), 0o600)
	res, err := Import(context.Background(), path, Options{})
	if err != nil || res.Kind != NUON || res.Sheet.Name() != "files" || shown(res.Sheet, addr(t, "B2")) != "1.0 kB" {
		t.Fatalf("import: %v %+v", err, res)
	}
	out := filepath.Join(dir, "out.json")
	if _, err := Export(context.Background(), out, JSON, Snap(res.Sheet, sheet.Rect{}, "files"), ExportOptions{}); err != nil {
		t.Fatal(err)
	}
	back, err := Import(context.Background(), out, Options{})
	if err != nil || back.Kind != JSON || shown(back.Sheet, addr(t, "B2")) != "1000" {
		t.Errorf("JSON back: %v, B2 %q", err, shown(back.Sheet, addr(t, "B2")))
	}
}

// FuzzReadNUON holds the table reader to never panicking and to
// storing only what fits.
func FuzzReadNUON(f *testing.F) {
	for _, s := range []string{`[[a, b]; [1, 2kb]]`, `[{a: [1 2]}, {b: 2026-09-27T11:27:31Z}]`, `{"a": 1}` + "\n" + `{"a": 2}`, "a,b\n1,2"} {
		f.Add([]byte(s))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		res, err := ImportReader(context.Background(), "stdin", bytes.NewReader(data), Options{MaxCells: 1000})
		if err == nil && res.Sheet.Len() > 1000 {
			t.Fatalf("%d cells", res.Sheet.Len())
		}
	})
}
