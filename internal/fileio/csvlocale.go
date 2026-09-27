package fileio

import (
	"bytes"
	"encoding/csv"

	"github.com/FelineStateMachine/012/internal/locale"
	"github.com/FelineStateMachine/012/internal/value"
)

// Delimited files are read and written in the importing workbook's
// locale, as Sheets does: 1,5 is a number in a CSV file read in de-DE.
// A file whose numbers are written with the other decimal separator is
// noticed from its sample and read that way, keeping the locale's date
// order and currency; and in a locale with a decimal comma, CSV files
// are written with ; between fields, as its spreadsheets write them.

// numberSample is how many fields of the sample decide how numbers are
// written.
const numberSample = 2000

// numberLocale is the locale a delimited file's fields are read in: loc,
// or loc with the other decimal separator (1.5 and 1,5) when at least
// two of the sample's fields, and more than twice as many as the other
// way round, read as numbers only that way (a lone 1,5 may be text).
// other reports the second.
func numberLocale(sample []byte, comma rune, loc *locale.Locale) (read *locale.Locale, other bool) {
	loc = loc.Or()
	alt := swapDecimal(loc)
	r := csv.NewReader(bytes.NewReader(sample))
	r.Comma, r.LazyQuotes, r.FieldsPerRecord, r.ReuseRecord = comma, true, -1, true
	mine, theirs := 0, 0
	for seen := 0; seen < numberSample; {
		rec, err := r.Read()
		if err != nil {
			break
		}
		for _, f := range rec {
			seen++
			_, _, inMine := value.ParseValueIn(f, loc)
			_, _, inTheirs := value.ParseValueIn(f, alt)
			switch {
			case inMine && !inTheirs:
				mine++
			case inTheirs && !inMine:
				theirs++
			}
		}
	}
	if theirs >= 2 && theirs > 2*mine {
		return alt, true
	}
	return loc, false
}

// swapDecimal is loc with the other decimal separator, grouping with the
// one it gave up: a comma for a point, a point for a comma.
func swapDecimal(loc *locale.Locale) *locale.Locale {
	alt := *loc
	if loc.Decimal == ',' {
		alt.Decimal, alt.Group = '.', ","
	} else {
		alt.Decimal, alt.Group = ',', "."
		if alt.DateSep == '.' {
			alt.DateSep = '/'
		}
	}
	return &alt
}

// csvComma separates the fields of a CSV file written in loc: ; where
// the decimal separator is a comma.
func csvComma(loc *locale.Locale) rune {
	if loc.Or().Decimal == ',' {
		return ';'
	}
	return ','
}

// decimalNote says how a file's numbers were read when not in the
// locale's own way.
func decimalNote(loc *locale.Locale) string {
	if loc.Decimal == ',' {
		return "numbers with decimal commas"
	}
	return "numbers with decimal points"
}
