package numfmt

import (
	"math"
	"time"
)

// Dates are serial numbers as in Sheets: whole days since 1899-12-30
// (day 0), with the time of day as the fraction. Unlike Excel there is no
// fictitious 1900-02-29, and dates before 1900 are negative.

var (
	MonthNames = [...]string{"January", "February", "March", "April", "May", "June", "July",
		"August", "September", "October", "November", "December"}
	DayNames = [...]string{"Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"}
)

// epochDays is 1899-12-30 as days since 0000-03-01 (see daysFromCivil).
var epochDays = daysFromCivil(1899, 12, 30)

// daysFromCivil counts days since 0000-03-01 in the proleptic Gregorian
// calendar (Howard Hinnant's algorithm). Months outside 1-12 normalize.
func daysFromCivil(y, m, d int) int {
	y, m = AddMonths(y, m, 0)
	if m <= 2 {
		y--
	}
	era := floorDivInt(y, 400)
	yoe := y - era*400
	mp := (m + 9) % 12
	doy := (153*mp+2)/5 + d - 1
	doe := yoe*365 + yoe/4 - yoe/100 + doy
	return era*146097 + doe
}

// Civil converts a serial day number to year, month and day.
func Civil(serial int64) (y, m, d int) {
	z := int(serial) + epochDays
	era := floorDivInt(z, 146097)
	doe := z - era*146097
	yoe := (doe - doe/1460 + doe/36524 - doe/146096) / 365
	y = yoe + era*400
	doy := doe - (365*yoe + yoe/4 - yoe/100)
	mp := (5*doy + 2) / 153
	d = doy - (153*mp+2)/5 + 1
	m = (mp+2)%12 + 1
	if m <= 2 {
		y++
	}
	return y, m, d
}

// DateSerial returns the serial of a date; month and day overflow into
// the next month or year, as DATE() allows.
func DateSerial(y, m, d int) float64 {
	return float64(daysFromCivil(y, m, 1) + d - 1 - epochDays)
}

// TimeSerial is the fraction of a day for a time of day.
func TimeSerial(h, m int, s float64) float64 {
	return (float64(h)*3600 + float64(m)*60 + s) / 86400
}

// AddMonths adds n months to month m of year y, returning a month from
// 1 to 12 and the year it falls in. m itself may be out of range.
func AddMonths(y, m, n int) (int, int) {
	m += n
	y += floorDivInt(m-1, 12)
	m = ((m-1)%12+12)%12 + 1
	return y, m
}

// DaysIn is the number of days in month m of year y.
func DaysIn(y, m int) int {
	return int(DateSerial(y, m+1, 1) - DateSerial(y, m, 1))
}

// Weekday returns 0 for Sunday through 6 for Saturday. Day 0 was a
// Saturday.
func Weekday(serial int64) int {
	return int(((serial+6)%7 + 7) % 7)
}

// SerialOf converts a time to a serial in its own time zone.
func SerialOf(t time.Time) float64 {
	h, m, s := t.Clock()
	return DateSerial(t.Year(), int(t.Month()), t.Day()) +
		TimeSerial(h, m, float64(s)+float64(t.Nanosecond())/1e9)
}

// SplitSerial returns the date and time parts of a serial, rounded to
// the second.
func SplitSerial(v float64) (days int64, secs int) {
	total := int64(math.Round(v * 86400))
	days = floorDiv(total, 86400)
	return days, int(total - days*86400)
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func floorDivInt(a, b int) int { return int(floorDiv(int64(a), int64(b))) }
