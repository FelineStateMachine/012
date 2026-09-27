package sheet

import (
	"testing"
	"time"

	"github.com/FelineStateMachine/012/internal/value"
)

func TestDateFunctions(t *testing.T) {
	checkFormulas(t, fixture(t), []fnCase{
		{"=DATE(2026, 9, 26)", num(46291)},
		{"=DATE(2026, 14, 1)", num(46419)}, // month 14 rolls into 2027
		{"=DATE(2026, 1, 0)", num(46022)},  // day 0 is the last of December
		{"=DATE(26, 1, 1)", num(9498)},     // two-digit years are 19xx
		{"=DATE(-1, 1, 1)", ErrNum},
		{"=DATE(1899, 12, 30)", num(693961)}, // 1899 counts as 3799, as in Sheets
		{"=TIME(14, 30, 0)", num(14.5 / 24)},
		{"=TIME(25, 0, 0)", num(1.0 / 24)},
		{"=TIME(0, -1, 0)", ErrNum},
		{"=TODAY()", num(46291)},
		{"=NOW()", num(46291 + 14.5/24)},
		{"=YEAR(F1)", num(2026)},
		{"=MONTH(F1)", num(9)},
		{"=DAY(F1)", num(26)},
		{`=YEAR("2026-09-26")`, num(2026)},
		{"=YEAR(-1)", ErrNum},
		{"=YEAR(A4)", ErrValue},
		{"=WEEKDAY(F1)", num(7)}, // Saturday
		{"=WEEKDAY(F1, 2)", num(6)},
		{"=WEEKDAY(F1, 3)", num(5)},
		{"=WEEKDAY(F1, 9)", ErrNum},
		{"=HOUR(F3)", num(14)},
		{"=MINUTE(F3)", num(30)},
		{"=SECOND(TIME(1, 2, 3))", num(3)},
		{"=HOUR(0.99999999)", num(0)}, // rounds to midnight
		{"=EDATE(F2, 1)", num(46081)}, // Jan 31 + 1 month is Feb 28
		{"=EDATE(F1, -12)", num(45926)},
		{"=EOMONTH(F1, 0)", num(46295)},
		{"=EOMONTH(F4, 12)", num(45716)},
		{`=DATEDIF(F4, F1, "Y")`, num(2)},
		{`=DATEDIF(F4, F1, "M")`, num(30)},
		{`=DATEDIF(F4, F1, "D")`, num(46291 - 45351)},
		{`=DATEDIF(F4, F1, "YM")`, num(6)},
		{`=DATEDIF(F4, F1, "MD")`, num(28)},
		{`=DATEDIF(F4, F1, "YD")`, num(210)}, // from Feb 28, the leap day's anniversary
		{`=DATEDIF(F2, F5, "D")`, ErrNum},    // start after end
		{`=DATEDIF(F4, F1, "Q")`, ErrNum},
		{"=DAYS(F1, F2)", num(238)},
		{`=DAYS("2026-01-02", "2026-01-01")`, num(1)},
		{"=NETWORKDAYS(F5, F2)", num(23)},
		{"=NETWORKDAYS(F2, F5)", num(-23)},
		{"=NETWORKDAYS(F5, F2, F5)", num(22)},
		{`=DATEVALUE("9/26/2026")`, num(46291)},
		{`=DATEVALUE("Sep 26, 2026 14:30")`, num(46291)},
		{`=DATEVALUE("hello")`, ErrValue},
		{"=DATEVALUE(46291)", ErrValue},
		{`=TIMEVALUE("2:30 PM")`, num(14.5 / 24)},
		{"=F1+1", num(46292)},
		{`="2026-09-27"-F1`, num(1)}, // date text coerces in arithmetic
	})
}

func TestInferredFormats(t *testing.T) {
	fixedNow(t)
	s := New()
	for a, in := range map[string]string{
		"A1": "$1,200.00", "A2": "$300.00", "A3": "9/26/2026", "A4": "10%", "A5": "9/1/2026",
	} {
		s.Set(at(a), in)
	}
	tests := []struct{ in, want string }{
		{"=A1+A2", "$1,500.00"},
		{"=SUM(A1:A2)", "$1,500.00"},
		{"=A1*2", "$2,400.00"},
		{"=A3+7", "10/3/2026"},
		{"=A3-A5", "25"},
		{"=DATE(2026,1,31)", "1/31/2026"},
		{"=EDATE(A3,1)", "10/26/2026"},
		{"=TODAY()", "9/26/2026"},
		{"=NOW()", "9/26/2026 14:30:00"},
		{"=A1*A4", "$120.00"}, // the first formatted input wins
		{"=YEAR(A3)", "2026"},
		{"=MAX(A3,A5)", "9/26/2026"},
		{"=IF(A1>0,A2,0)", "$300.00"},
	}
	for _, tt := range tests {
		s.Set(at("C1"), tt.in)
		if got, _ := Display(s.Value(at("C1")), s.DisplayFormat(at("C1")), 30); got != tt.want {
			t.Errorf("%s shows %q, want %q", tt.in, got, tt.want)
		}
	}
	// Changing an input's format updates formulas that follow it.
	s.Set(at("C1"), "=A1+A2")
	s.SetFormat(NewRect(at("A1"), at("A1")), Preset(FmtNumber))
	if got, _ := Display(s.Value(at("C1")), s.DisplayFormat(at("C1")), 30); got != "1,500.00" {
		t.Errorf("after reformat C1 shows %q", got)
	}
}

func TestVolatileRecalc(t *testing.T) {
	fixedNow(t)
	s := New()
	s.Set(at("A1"), "=TODAY()")
	s.Set(at("A2"), "=A1+1")
	if got := s.Value(at("A2")).Num; got != 46292 {
		t.Fatalf("A2 = %v", got)
	}
	value.Now = func() time.Time { return time.Date(2026, 9, 27, 9, 0, 0, 0, time.Local) }
	s.Set(at("Z1"), "unrelated") // any change recalculates volatile cells
	if got := s.Value(at("A2")).Num; got != 46293 {
		t.Errorf("A2 after a day = %v, want 46293", got)
	}
	s.Set(at("A1"), "5")
	if _, ok := s.volatile[at("A1")]; ok {
		t.Error("A1 still volatile after replacing the formula")
	}
}

func TestVolatileFunctions(t *testing.T) {
	for _, name := range []string{"TODAY", "NOW", "RAND", "RANDBETWEEN"} {
		if f, _ := LookupFunc(name); !f.Volatile {
			t.Errorf("%s not volatile", name)
		}
	}
}
