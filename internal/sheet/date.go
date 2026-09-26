package sheet

import (
	"math"
	"strconv"
	"strings"
	"time"
)

// Dates are serial numbers as in Sheets: whole days since 1899-12-30
// (day 0), with the time of day as the fraction. Unlike Excel there is no
// fictitious 1900-02-29, and dates before 1900 are negative.

// Now is the clock used by TODAY() and NOW(); tests replace it.
var Now = time.Now

// epochDays is 1899-12-30 as days since 0000-03-01 (see daysFromCivil).
var epochDays = daysFromCivil(1899, 12, 30)

// daysFromCivil counts days since 0000-03-01 in the proleptic Gregorian
// calendar (Howard Hinnant's algorithm). Months outside 1-12 normalize.
func daysFromCivil(y, m, d int) int {
	y += floorDivInt(m-1, 12)
	m = (m-1)%12 + 1
	if m <= 0 {
		m += 12
	}
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

// civil converts a serial day number to year, month and day.
func civil(serial int64) (y, m, d int) {
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

// dateSerial returns the serial of a date; month and day overflow into
// the next month or year, as DATE() allows.
func dateSerial(y, m, d int) float64 {
	return float64(daysFromCivil(y, m, 1) + d - 1 - epochDays)
}

// timeSerial is the fraction of a day for a time of day.
func timeSerial(h, m int, s float64) float64 {
	return (float64(h)*3600 + float64(m)*60 + s) / 86400
}

// weekday returns 0 for Sunday through 6 for Saturday. Day 0 was a
// Saturday.
func weekday(serial int64) int {
	return int(((serial+6)%7 + 7) % 7)
}

// serialOf converts a time to a serial in its own time zone.
func serialOf(t time.Time) float64 {
	h, m, s := t.Clock()
	return dateSerial(t.Year(), int(t.Month()), t.Day()) +
		timeSerial(h, m, float64(s)+float64(t.Nanosecond())/1e9)
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if (a%b != 0) && ((a < 0) != (b < 0)) {
		q--
	}
	return q
}

func floorDivInt(a, b int) int { return int(floorDiv(int64(a), int64(b))) }

// splitSerial returns the date and time parts of a serial, rounded to
// the second.
func splitSerial(v float64) (days int64, secs int) {
	total := int64(math.Round(v * 86400))
	days = floorDiv(total, 86400)
	return days, int(total - days*86400)
}

// parseDateTime recognizes dates and times typed into a cell the way
// Sheets does in the en-US locale: 9/26/2026, 9/26/26, 9/26 (this year),
// 2026-09-26, Sep 26, 2026, 26 Sep 2026, 14:30, 2:30 PM, 2pm, 25:30 (a
// duration) and a date followed by a time. It returns the serial and the
// format Sheets would apply.
func parseDateTime(s string) (float64, Format, bool) {
	s = strings.TrimSpace(s)
	if s == "" || !(isDigit(s[0]) || isLetter(s[0])) {
		return 0, Format{}, false
	}
	if t, f, ok := parseTime(s); ok {
		return t, f, true
	}
	// A date, optionally followed by a time after the last space.
	if d, f, ok := parseDate(s); ok {
		return d, f, true
	}
	if i := strings.LastIndexByte(s, ' '); i > 0 {
		datePart, timePart := s[:i], s[i+1:]
		// "2:30 PM" splits the time; move the meridiem back.
		if up := strings.ToUpper(timePart); up == "AM" || up == "PM" {
			if j := strings.LastIndexByte(datePart, ' '); j > 0 {
				datePart, timePart = s[:j], s[j+1:]
			}
		}
		d, _, ok1 := parseDate(datePart)
		t, tf, ok2 := parseTime(timePart)
		if ok1 && ok2 && tf.Kind == FmtTime {
			f := Preset(FmtDateTime)
			if strings.Contains(strings.ToUpper(timePart), "M") {
				f.Pattern = "m/d/yyyy h:mm:ss am/pm"
			}
			return d + t, f, true
		}
	}
	return 0, Format{}, false
}

// parseTime recognizes H:MM, H:MM:SS(.fff) and H AM/PM forms. Hours of
// 24 or more make a duration.
func parseTime(s string) (float64, Format, bool) {
	up := strings.ToUpper(strings.TrimSpace(s))
	meridiem := ""
	for _, m := range []string{"AM", "PM"} {
		if strings.HasSuffix(up, m) {
			meridiem, up = m[:1], strings.TrimSpace(strings.TrimSuffix(up, m))
			break
		}
	}
	parts := strings.Split(up, ":")
	if len(parts) > 3 || (len(parts) == 1 && meridiem == "") {
		return 0, Format{}, false
	}
	var h, m int
	var sec float64
	for i, p := range parts {
		if p == "" || !isDigit(p[0]) {
			return 0, Format{}, false
		}
		if i == 2 {
			v, err := strconv.ParseFloat(p, 64)
			if err != nil || v >= 60 || strings.ContainsAny(p, "eE+-") {
				return 0, Format{}, false
			}
			sec = v
			continue
		}
		if len(p) > 2 && i > 0 || strings.Trim(p, "0123456789") != "" {
			return 0, Format{}, false
		}
		n, _ := strconv.Atoi(p)
		if i == 0 {
			h = n
		} else {
			if n >= 60 || len(p) != 2 {
				return 0, Format{}, false
			}
			m = n
		}
	}
	f := Format{Kind: FmtTime, Pattern: "h:mm:ss"}
	switch {
	case meridiem != "":
		if h < 1 || h > 12 {
			return 0, Format{}, false
		}
		h %= 12
		if meridiem == "P" {
			h += 12
		}
		f = Preset(FmtTime)
	case h >= 24:
		f = Preset(FmtDuration)
	}
	return timeSerial(h, m, sec), f, true
}

// parseDate recognizes the date forms listed at parseDateTime.
func parseDate(s string) (float64, Format, bool) {
	s = strings.TrimSpace(s)
	switch {
	case strings.Count(s, "/") >= 1 && strings.Trim(s, "0123456789/") == "":
		p := strings.Split(s, "/")
		if len(p[0]) == 4 && len(p) == 3 { // 2026/09/26
			return ymd(p[0], p[1], p[2], Format{Kind: FmtDate, Pattern: "yyyy/mm/dd"})
		}
		switch len(p) {
		case 2:
			return ymd(strconv.Itoa(Now().Year()), p[0], p[1], Preset(FmtDate))
		case 3:
			return ymd(p[2], p[0], p[1], Preset(FmtDate))
		}
	case strings.Count(s, "-") == 2 && strings.Trim(s, "0123456789-") == "":
		p := strings.Split(s, "-")
		if len(p[0]) == 4 { // ISO 2026-09-26
			return ymd(p[0], p[1], p[2], Format{Kind: FmtDate, Pattern: "yyyy-mm-dd"})
		}
		return ymd(p[2], p[0], p[1], Preset(FmtDate))
	}
	// Month names: "Sep 26, 2026", "September 26 2026", "26 Sep 2026", "Sep 26".
	words := strings.Fields(strings.ReplaceAll(s, ",", " "))
	if len(words) < 2 || len(words) > 3 {
		return 0, Format{}, false
	}
	year := strconv.Itoa(Now().Year())
	if len(words) == 3 {
		year = words[2]
	}
	if m, long, ok := monthName(words[0]); ok {
		f := Format{Kind: FmtDate, Pattern: "mmm d, yyyy"}
		if long {
			f.Pattern = "mmmm d, yyyy"
		}
		return ymd(year, strconv.Itoa(m), words[1], f)
	}
	if m, long, ok := monthName(words[1]); ok {
		f := Format{Kind: FmtDate, Pattern: "d mmm yyyy"}
		if long {
			f.Pattern = "d mmmm yyyy"
		}
		return ymd(year, strconv.Itoa(m), words[0], f)
	}
	return 0, Format{}, false
}

// monthName matches a month's full name or three-letter abbreviation.
func monthName(w string) (month int, long, ok bool) {
	w = strings.ToLower(strings.TrimSuffix(w, "."))
	for i, n := range monthNames {
		n = strings.ToLower(n)
		if w == n {
			return i + 1, len(w) > 3, true
		}
		if w == n[:3] {
			return i + 1, false, true
		}
	}
	return 0, false, false
}

// ymd validates and converts date parts. Two-digit years 00-29 are
// 2000-2029 and 30-99 are 1930-1999, as in Sheets.
func ymd(ys, ms, ds string, f Format) (float64, Format, bool) {
	y, err1 := strconv.Atoi(ys)
	m, err2 := strconv.Atoi(ms)
	d, err3 := strconv.Atoi(ds)
	if err1 != nil || err2 != nil || err3 != nil || len(ys) == 3 || len(ys) > 4 || len(ms) > 2 || len(ds) > 2 {
		return 0, Format{}, false
	}
	if len(ys) <= 2 {
		if y < 30 {
			y += 2000
		} else {
			y += 1900
		}
	}
	if m < 1 || m > 12 || d < 1 || d > daysIn(y, m) {
		return 0, Format{}, false
	}
	return dateSerial(y, m, d), f, true
}

func daysIn(y, m int) int {
	return int(dateSerial(y, m+1, 1) - dateSerial(y, m, 1))
}
