package fileio

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// The Sheets corpus (testdata/sheets_corpus.tsv) holds formulas whose
// results 012 matches with Google Sheets on purpose, each with 012's
// result and Sheets' as checked in Sheets. TestSheetsCorpus holds 012 to
// its recorded results and asks every difference from Sheets for a
// reason. The corpus reaches Sheets as an XLSX workbook written by 012's
// own exporter (sheetscorpus_xlsx_test.go), and Sheets' answers come back
// as a CSV download of its corpus sheet.
var (
	updateCorpus = flag.Bool("update-corpus", false, "record 012's current results in testdata/sheets_corpus.tsv")
	corpusXLSX   = flag.String("corpus-xlsx", "", "write the Sheets corpus as an XLSX workbook to this path, to import into Sheets")
	corpusFrom   = flag.String("update-from", "", "record Sheets' results from a CSV download of the imported corpus sheet")
)

const (
	corpusPath  = "testdata/sheets_corpus.tsv"
	unconfirmed = "unconfirmed" // Sheets' result, until checked in Sheets
	onPurpose   = "on purpose:" // starts the note of a deliberate difference
)

// corpusFixture is the Data sheet the corpus formulas read, typed as a
// user would: the oracle's fixture, with a text number, an error, a
// FALSE and a #N/A in column J.
//
//	   A      B      C  D       E    F           G  H      I   J
//	1  10     north  1  Apple   1.5  9/26/2026   1  -1000  30  '3
//	2  20     south  2  Banana  2.5  2026-01-31  2  300    20  =1/0
//	3  30     north  3  Cherry  3.5  14:30       2  400    10  FALSE
//	4  apple  east   4  date    x    2/29/2024   3  500        =NA()
//	5         north  5  Apple   9    12/31/2025     5
//	6         =""    6          TRUE
//	7  -5     Ab*d   7
var corpusFixture = map[string]string{
	"A1": "10", "A2": "20", "A3": "30", "A4": "apple", "A7": "-5",
	"B1": "north", "B2": "south", "B3": "north", "B4": "east", "B5": "north", "B6": `=""`, "B7": "Ab*d",
	"C1": "1", "C2": "2", "C3": "3", "C4": "4", "C5": "5", "C6": "6", "C7": "7",
	"D1": "Apple", "D2": "Banana", "D3": "Cherry", "D4": "date", "D5": "Apple",
	"E1": "1.5", "E2": "2.5", "E3": "3.5", "E4": "x", "E5": "9", "E6": "TRUE",
	"F1": "9/26/2026", "F2": "2026-01-31", "F3": "14:30", "F4": "2/29/2024", "F5": "12/31/2025",
	"G1": "1", "G2": "2", "G3": "2", "G4": "3",
	"H1": "-1000", "H2": "300", "H3": "400", "H4": "500",
	"I1": "30", "I2": "20", "I3": "10",
	"J1": "'3", "J2": "=1/0", "J3": "FALSE", "J4": "=NA()",
}

// The corpus sheet's columns: each formula is in C, and D and E read its
// result as text and its kind with formulas both 012 and Sheets compute,
// so both answer in the same terms.
const (
	colSection = iota
	colFormula
	colResult
	colText
	colKind
	colOurs
	colCheck
	colNote
)

var corpusHeaders = []string{"section", "formula", "result", "as text", "kind", "012 result", "check", "note"}

// corpusLine is a line of the corpus file: a formula row, or a comment,
// section heading or blank line kept as it is.
type corpusLine struct {
	text    string
	section string
	formula string
	ours    string // 012's recorded result
	sheets  string // Sheets' result, or unconfirmed
	note    string
}

func readCorpus(t *testing.T) []*corpusLine {
	t.Helper()
	data, err := os.ReadFile(corpusPath)
	if err != nil {
		t.Fatal(err)
	}
	var out []*corpusLine
	section := ""
	for _, text := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if !strings.HasPrefix(text, "=") {
			if h, ok := strings.CutPrefix(text, "## "); ok {
				section = h
			}
			out = append(out, &corpusLine{text: text})
			continue
		}
		f := append(strings.Split(text, "\t"), "", "", "")
		out = append(out, &corpusLine{section: section, formula: f[0], ours: f[1], sheets: f[2], note: f[3]})
	}
	return out
}

func writeCorpus(t *testing.T, lines []*corpusLine) {
	t.Helper()
	var b strings.Builder
	for _, l := range lines {
		if l.formula == "" {
			b.WriteString(l.text + "\n")
			continue
		}
		if l.sheets == "" {
			l.sheets = unconfirmed
		}
		b.WriteString(strings.TrimRight(strings.Join([]string{l.formula, l.ours, l.sheets, l.note}, "\t"), "\t") + "\n")
	}
	if err := os.WriteFile(corpusPath, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

// corpusRows are the formula rows of lines, in order.
func corpusRows(lines []*corpusLine) []*corpusLine {
	var rows []*corpusLine
	for _, l := range lines {
		if l.formula != "" {
			rows = append(rows, l)
		}
	}
	return rows
}

// corpusBook builds the workbook the corpus is computed in: the Corpus
// sheet, a row per formula under a header row, and the Data sheet.
func corpusBook(t *testing.T, rows []*corpusLine) *sheet.Sheet {
	t.Helper()
	s := sheet.New()
	book := s.Book()
	if err := book.RenameSheet(s, "Corpus"); err != nil {
		t.Fatal(err)
	}
	data, err := book.AddSheet("Data", 1)
	if err != nil {
		t.Fatal(err)
	}
	for a, in := range corpusFixture {
		at := addr(t, a)
		setAt(t, data, at.Col, at.Row, in)
	}
	for c, h := range corpusHeaders {
		setAt(t, s, c, 0, "'"+h)
	}
	for i, r := range rows {
		n := strconv.Itoa(i + 2)
		setAt(t, s, colSection, i+1, r.section)
		setAt(t, s, colFormula, i+1, "'"+r.formula)
		setAt(t, s, colResult, i+1, r.formula)
		setAt(t, s, colText, i+1, `=C`+n+`&""`)
		setAt(t, s, colKind, i+1, kindFormula("C"+n))
	}
	for c, w := range []int{22, 48, 14, 24, 8, 24, 8, 30} {
		s.SetColWidth(c, w)
	}
	return s
}

// kindFormula names the kind of the value in ref, in words both 012 and
// Sheets compute alike.
func kindFormula(ref string) string {
	return fmt.Sprintf(`=IF(ISERROR(%[1]s), "error", IF(ISNUMBER(%[1]s), "number", IF(ISTEXT(%[1]s), "text", `+
		`IF(ISLOGICAL(%[1]s), "logical", IF(ISBLANK(%[1]s), "blank", "other")))))`, ref)
}

// setAt types in into the cell at col and row of s.
func setAt(t *testing.T, s *sheet.Sheet, col, row int, in string) {
	t.Helper()
	a := sheet.Addr{Col: col, Row: row}
	if err := s.Set(a, in); err != nil {
		t.Errorf("%s %q: %v", a, in, err)
	}
}

// corpusResult reads what 012 computed for the formula on row i of the
// corpus sheet, in the corpus file's terms.
func corpusResult(s *sheet.Sheet, i int) string {
	text := s.Value(sheet.Addr{Col: colText, Row: i + 1})
	kind := s.Value(sheet.Addr{Col: colKind, Row: i + 1})
	return encodeResult(text.String(), kind.String())
}

// encodeResult writes a result, given as text and its kind, as the
// corpus file does: text quoted, (blank) for an empty cell, and
// numbers, booleans and errors as they read.
func encodeResult(text, kind string) string {
	switch kind {
	case "number", "logical", "error":
		return text
	case "text":
		return strconv.Quote(text)
	case "blank":
		return "(blank)"
	}
	return "(" + kind + ") " + text
}

func TestSheetsCorpus(t *testing.T) {
	lines := readCorpus(t)
	rows := corpusRows(lines)
	s := corpusBook(t, rows)
	if *corpusFrom != "" {
		foldSheets(t, rows, *corpusFrom)
	}
	for i, r := range rows {
		got := corpusResult(s, i)
		if *updateCorpus {
			r.ours = got
		}
		checkCorpusRow(t, r, got)
	}
	if *updateCorpus || *corpusFrom != "" {
		writeCorpus(t, lines)
	}
	if *corpusXLSX != "" {
		exportCorpus(t, s, rows, *corpusXLSX)
	}
}

// checkCorpusRow holds 012 to its recorded result, and a difference
// from Sheets to a note saying why.
func checkCorpusRow(t *testing.T, r *corpusLine, got string) {
	t.Helper()
	if got != r.ours {
		t.Errorf("%s = %s, recorded %s (go test ./internal/fileio -run SheetsCorpus -update-corpus records it)", r.formula, got, r.ours)
	}
	if r.sheets != "" && r.sheets != unconfirmed && r.sheets != r.ours && !strings.HasPrefix(r.note, onPurpose) {
		t.Errorf("%s = %s in 012 and %s in Sheets: make 012 agree, or say why in a note starting %q", r.formula, r.ours, r.sheets, onPurpose)
	}
}
