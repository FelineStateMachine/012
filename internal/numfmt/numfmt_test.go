package numfmt

import (
	"testing"

	"github.com/FelineStateMachine/012/internal/locale"
)

func TestFormat(t *testing.T) {
	tests := []struct {
		v    float64
		pat  string
		want string
	}{
		{1234.5, "#,##0.00", "1,234.50"},
		{-1234.5, "#,##0.00", "-1,234.50"},
		{0.5, "#,##0", "1"},
		{0, "#,##0.00", "0.00"},
		{1234567.891, "#,##0", "1,234,568"},
		{1.005, "0.00", "1.01"}, // 15 significant digits, not binary
		{2.5, "0", "3"},
		{-2.5, "0", "-3"},
		{-0.001, "0.00", "0.00"}, // no negative zero
		{0.1234, "0%", "12%"},
		{0.1234, "0.00%", "12.34%"},
		{1234.5, "0.00E+00", "1.23E+03"},
		{0.000123, "0.00E+00", "1.23E-04"},
		{9.999, "0.00E+00", "1.00E+01"},
		{0, "0.00E+00", "0.00E+00"},
		{1234.5, `"$"#,##0.00`, "$1,234.50"},
		{-1234.5, `"$"#,##0.00`, "-$1,234.50"},
		{-1234.5, "#,##0.00;(#,##0.00)", "(1,234.50)"},
		{0, "#,##0;(#,##0);\"zero\"", "zero"},
		{1.5, "0.0#", "1.5"},
		{1.25, "0.0#", "1.25"},
		{1.5, "0.??", "1.5 "},
		{5, "000", "005"},
		{12345, "#,##0,", "12"},
		{1234.5, "#,##0.00 \"kg\"", "1,234.50 kg"},
		{0.5, "#.##", ".5"},
		{12.5, "General", "12.5"},
		{5551234, "000-0000", "555-1234"},
		{1.5, "General.00", "1.5.00"}, // placeholders after General get zeros
		// Dates and times: 46291 is 2026-09-26, a Saturday.
		{46291, "m/d/yyyy", "9/26/2026"},
		{46291, "yyyy-mm-dd", "2026-09-26"},
		{46291, "mmm d, yyyy", "Sep 26, 2026"},
		{46291, "dddd, mmmm d", "Saturday, September 26"},
		{46291, "ddd mmmmm yy", "Sat S 26"},
		{46291.6041666667, "h:mm am/pm", "2:30 PM"},
		{46291.6041666667, "hh:mm:ss", "14:30:00"},
		{0.5, "h:mm:ss AM/PM", "12:00:00 PM"},
		{0, "h:mm a/p", "12:00 A"},
		{1.0423611111, "[h]:mm:ss", "25:01:00"},
		{-0.5, "[h]:mm", "-12:00"},
		{0.000011574, "h:mm:ss.00", "0:00:01.00"},
		{0.0000173611, "mm:ss.0", "00:01.5"},
		{46291.99999999, "m/d/yyyy h:mm", "9/27/2026 0:00"},
		{0, "m/d/yyyy", "12/30/1899"},
		{-1, "yyyy-mm-dd", "1899-12-29"},
		{61, "yyyy-mm-dd", "1900-03-01"},
		{2958465, "yyyy-mm-dd", "9999-12-31"},
	}
	for _, tt := range tests {
		if got := Format(tt.v, tt.pat); got != tt.want {
			t.Errorf("Format(%v, %q) = %q, want %q", tt.v, tt.pat, got, tt.want)
		}
	}
}

func TestAdjustDecimals(t *testing.T) {
	for _, tt := range []struct{ pat, want string }{
		{"0.00", "0.000"}, {"#,##0", "#,##0.0"}, {`"$"#,##0.00_);("$"#,##0.00)`, `"$"#,##0.000_);("$"#,##0.000)`},
		{"0.00E+00", "0.000E+00"}, {"0%", "0.0%"},
	} {
		if got := AdjustDecimals(tt.pat, 1); got != tt.want {
			t.Errorf("AdjustDecimals(%q, 1) = %q, want %q", tt.pat, got, tt.want)
		}
	}
	if got := AdjustDecimals("0.0", -1); got != "0" {
		t.Errorf("AdjustDecimals(0.0, -1) = %q", got)
	}
}

// TestFormatNames checks month and day names follow the locale's
// language, in the form a date with a day takes where the language has
// one, and its words for AM and PM.
func TestFormatNames(t *testing.T) {
	const sat = 46291.6041666667 // 2026-09-26 14:30, a Saturday
	for _, tt := range []struct{ tag, pat, want string }{
		{"de-DE", "dddd, d. mmmm yyyy", "Samstag, 26. September 2026"},
		{"de-DE", "ddd d mmm", "Sa 26 Sep"},
		{"fr-FR", "dddd d mmmm", "samedi 26 septembre"},
		{"fr-FR", "d mmm yy", "26 sept. 26"},
		{"pl-PL", "d mmmm yyyy", "26 września 2026"},
		{"pl-PL", "mmmm yyyy", "wrzesień 2026"},
		{"ru-RU", "mmmmm", "С"},
		{"ja-JP", "h:mm AM/PM", "2:30 午後"},
		{"zh-CN", "dddd", "星期六"},
		{"en-GB", "ddd d mmm", "Sat 26 Sep"},
		{"de-DE", "h:mm AM/PM", "2:30 PM"},
	} {
		l, _ := locale.Lookup(tt.tag)
		if got := FormatIn(sat, tt.pat, l); got != tt.want {
			t.Errorf("%s FormatIn(%q) = %q, want %q", tt.tag, tt.pat, got, tt.want)
		}
	}
}

// TestFormatLCID checks a [$-407] tag shows its locale's names, as in
// Excel, whatever the locale shown in.
func TestFormatLCID(t *testing.T) {
	for _, tt := range []struct{ pat, want string }{
		{"[$-407]d. mmmm yyyy", "26. September 2026"},
		{"[$-40C]dddd", "samedi"},
		{"[$-10407]mmm", "Sep"},
		{"[$€-407]mmmm", "€September"},
		{"[$-F800]dddd", "Saturday"},
	} {
		if got := Format(46291, tt.pat); got != tt.want {
			t.Errorf("Format(%q) = %q, want %q", tt.pat, got, tt.want)
		}
	}
}
