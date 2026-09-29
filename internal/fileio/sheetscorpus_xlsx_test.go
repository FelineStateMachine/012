package fileio

import (
	"context"
	"encoding/csv"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/FelineStateMachine/012/internal/sheet"
)

// sheetsOnlyNote marks a corpus row whose formula 012's exporter saves as
// its value, because Excel has nothing the same (formula.go): in Sheets
// the formula has to be typed into C.
const sheetsOnlyNote = "saved as a value: type the formula in B into C"

// exportCorpus writes the corpus workbook s as an XLSX file at name with
// 012's exporter, for importing into Sheets. Sheets recalculates C, D
// and E on import; F keeps 012's results, and G compares the two.
func exportCorpus(t *testing.T, s *sheet.Sheet, rows []*corpusLine, name string) {
	t.Helper()
	for i, r := range rows {
		n := strconv.Itoa(i + 2)
		setAt(t, s, colOurs, i+1, "'"+corpusResult(s, i))
		setAt(t, s, colCheck, i+1, checkFormula(n))
		note := r.note
		if !excelKeeps(r.formula) {
			setAt(t, s, colResult, i+1, "")
			note = sheetsOnlyNote
		}
		setAt(t, s, colNote, i+1, note)
	}
	res, err := Export(context.Background(), name, XLSX, SnapBook(s), ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range res.Notes {
		t.Log(n)
	}
	t.Logf("wrote %d formulas to %s", len(rows), name)
}

// checkFormula compares, on row n, the result in D and E with 012's in
// F, writing it as encodeResult does. Errors only say both are errors;
// the error in C and the one in F are compared by eye.
func checkFormula(n string) string {
	d, e, f := "D"+n, "E"+n, "F"+n
	enc := `IF(` + e + `="text", """" & ` + d + ` & """", IF(` + e + `="blank", "(blank)", ` + d + ` & ""))`
	return `=IFERROR(IF(EXACT(` + enc + `, ` + f + `), "same", "DIFFERS"), IF(LEFT(` + f + `, 1)="#", "both errors", "DIFFERS"))`
}

// excelKeeps reports whether 012's exporter keeps formula as a formula.
func excelKeeps(formula string) bool {
	_, ok := toExcelFormula(formula, nil)
	return ok
}

// foldSheets records Sheets' results from name, a CSV download of the
// corpus sheet after importing the XLSX workbook into Sheets. Rows left
// out of the download, formulas saved as values that weren't typed in,
// and functions the import didn't recognize stay as they are.
func foldSheets(t *testing.T, rows []*corpusLine, name string) {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	recs, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) == 0 || !slices.Equal(recs[0][:colOurs], corpusHeaders[:colOurs]) {
		t.Fatalf("%s: expected a CSV download of the corpus sheet, with headers %q", name, corpusHeaders[:colOurs])
	}
	sheets := map[string]string{}
	for _, rec := range recs[1:] {
		if len(rec) > colKind {
			text := rec[colText]
			if rec[colKind] == "number" {
				// Sheets writes a number a hair off a whole one, such as
				// 100*1.1, as "110." when it rounds to 15 digits.
				text = strings.TrimSuffix(text, ".")
			}
			sheets[rec[colFormula]] = encodeResult(text, rec[colKind])
		}
	}
	skipped := 0
	for _, r := range rows {
		got, ok := sheets[r.formula]
		// #NAME? where 012 knows every name means the import didn't
		// recognize a function (Sheets reads Excel's _xlfn._xlws.FILTER
		// so): the formula wasn't computed.
		if !ok || (got == "(blank)" && !excelKeeps(r.formula)) || (got == "#NAME?" && r.ours != got) {
			skipped++
			continue
		}
		r.sheets = got
	}
	if skipped > 0 {
		t.Logf("%d formulas have no result from Sheets in %s", skipped, name)
	}
}
