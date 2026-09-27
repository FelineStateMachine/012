package e2e

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// germanBudget types a small budget in English (United States), then
// picks German (Germany) in File > Settings > Locale from the palette.
func germanBudget(s *session) {
	s.keys("Item", "<tab>", "Amount", "<tab>", "Due", "<enter>")
	s.keys("Rent", "<tab>", "$950.00", "<tab>", "9/26/2026", "<enter>")
	s.keys("Power", "<tab>", "$84.20", "<tab>", "10/3/2026", "<enter>")
	s.keys("Total", "<tab>", "=SUM(B2:B3)", "<enter>")
	s.keys("VAT", "<tab>", "=ROUND(B4*0.19, 2)", "<enter>")
	s.keys("<ctrl+k>", "locale", "<enter>")
	s.waitFor("Type a language, country or tag")
	s.keys("germany", "<enter>")
	s.waitFor("Locale German (Germany)")
}

// Switching the locale redraws the sheet German, takes entries and
// formulas typed the German way, saves them as en-US writes them, and
// undoes back to en-US.
func TestLocaleSwitch(t *testing.T) {
	dir := t.TempDir()
	s := start(t, dir)
	germanBudget(s)
	s.waitFor("1.034,20")
	s.waitFor("26.9.2026")
	s.keys("<up>", "<right>")
	s.waitForBar("B5", "=ROUND(B4*0,19; 2)")

	s.keys("<down>", "<left>", "Rate", "<tab>", "12,5 %", "<enter>")
	s.keys("Share", "<tab>", "=ROUND(B5/B4; ")
	s.waitFor("ROUND(value; [places])")
	s.keys("3)", "<enter>")
	s.keys("<up>", "<right>")
	s.waitForBar("B7", "=ROUND(B5/B4; 3)")
	s.waitFor("0,19")

	s.keys("<ctrl+s>", "budget", "<enter>")
	s.eventually("saved", func() bool { return s.title() == "012 - budget.012" })
	data, err := os.ReadFile(filepath.Join(dir, "budget.012"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"locale": "de-DE"`, `"=ROUND(B4*0.19, 2)"`, `"12.5%"`, `"=ROUND(B5/B4, 3)"`, `"9/26/2026"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("the file lacks %s:\n%s", want, data)
		}
	}

	// Undo the entries and the locale: the sheet is en-US's again.
	s.keys("<ctrl+z>", "<ctrl+z>", "<ctrl+z>", "<ctrl+z>", "<ctrl+z>")
	s.waitFor("$1,034.20")
	s.waitFor("9/26/2026")
}

// The config's locale, here from LANG, is the default of new sheets;
// a file saved without a locale of its own opens in the reader's.
func TestLocaleFromLANG(t *testing.T) {
	dir := t.TempDir()
	s := startWith(t, options{dir: dir, env: []string{"LANG=de_DE.UTF-8"}})
	s.keys("1,5", "<enter>", "=A1*2", "<enter>")
	s.keys("<up>")
	s.waitForBar("A2", "=A1*2")
	s.keys("<up>")
	s.waitForBar("A1", "1,5")
	s.keys("<ctrl+s>", "half", "<enter>")
	s.eventually("saved", func() bool { return s.title() == "012 - half.012" })
	s.keys("<ctrl+q>")
	s.waitExit()

	r := start(t, dir, "half.012")
	r.waitForBar("A1", "1.5")
}

// In German, a date typed with a German month name is a date, shown
// with German names, and a formula that doesn't parse names German's
// separator.
func TestLocaleNamesAndErrors(t *testing.T) {
	s := start(t, t.TempDir())
	germanBudget(s)
	s.keys("<ctrl+g>", "D1", "<enter>", "3. Okt. 2026", "<enter>")
	s.waitFor("3 Okt 2026")
	s.keys("=SUM(1;2 3)", "<enter>")
	s.waitFor("Expected ; or ) in SUM")
}
