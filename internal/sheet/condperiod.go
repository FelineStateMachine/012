package sheet

import (
	"strings"

	"github.com/FelineStateMachine/012/internal/numfmt"
	"github.com/FelineStateMachine/012/internal/value"
)

// Date rules ("Date is", "is before", "is after") take a date or a
// period as Sheets and Excel name them: today, tomorrow, yesterday, the
// past week, month or year, and this, last or next week, month or year.
// Weeks run Sunday to Saturday, as Excel's this week does. A period is
// a range of days: a date is in it, before its first day or after its
// last.

// Periods lists the words a date rule takes, in the order the rules
// editor's hint gives them.
func Periods() []string {
	return []string{"today", "tomorrow", "yesterday", "past week", "past month", "past year",
		"this week", "last week", "next week", "this month", "last month", "next month",
		"this year", "last year", "next year"}
}

// datePeriod reads a date rule's value: a date as typed, a number of
// days, or a period, as the first and last days it covers.
func datePeriod(arg string) (lo, hi float64, ok bool) {
	today := float64(int64(numfmt.SerialOf(value.Now())))
	word := strings.Join(strings.Fields(strings.ToLower(arg)), " ")
	word = strings.TrimPrefix(strings.TrimPrefix(word, "in the "), "the ")
	switch word {
	case "today":
		return today, today, true
	case "tomorrow":
		return today + 1, today + 1, true
	case "yesterday":
		return today - 1, today - 1, true
	case "past week", "last 7 days":
		return today - 6, today, true
	case "past month":
		return monthsBefore(today, 1), today, true
	case "past year":
		return monthsBefore(today, 12), today, true
	}
	if which, unit, found := strings.Cut(word, " "); found {
		step, known := map[string]int{"this": 0, "last": -1, "next": 1}[which]
		if lo, hi, ok := periodOf(today, unit, step); known && ok {
			return lo, hi, true
		}
	}
	n, _, ok := ParseValue(strings.TrimSpace(arg))
	n = float64(int64(n))
	return n, n, ok
}

// periodOf is the week, month or year step periods away from the one
// holding day.
func periodOf(day float64, unit string, step int) (lo, hi float64, ok bool) {
	y, m, _ := numfmt.Civil(int64(day))
	switch unit {
	case "week":
		lo = day - float64(numfmt.Weekday(int64(day))) + float64(7*step)
		return lo, lo + 6, true
	case "month":
		y, m = numfmt.AddMonths(y, m, step)
		return numfmt.DateSerial(y, m, 1), numfmt.DateSerial(y, m, numfmt.DaysIn(y, m)), true
	case "year":
		return numfmt.DateSerial(y+step, 1, 1), numfmt.DateSerial(y+step, 12, 31), true
	}
	return 0, 0, false
}

// monthsBefore is the day n months before day, on the last day of that
// month when it's shorter.
func monthsBefore(day float64, n int) float64 {
	y, m, d := numfmt.Civil(int64(day))
	y, m = numfmt.AddMonths(y, m, -n)
	return numfmt.DateSerial(y, m, min(d, numfmt.DaysIn(y, m)))
}

// IsPeriod reports whether a date rule's value names a period (or a day
// relative to today) rather than a date.
func IsPeriod(arg string) bool {
	_, _, ok := datePeriod(arg)
	_, _, isDate := ParseValue(strings.TrimSpace(arg))
	return ok && !isDate
}
