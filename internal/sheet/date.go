package sheet

import (
	"strconv"
	"strings"
	"time"

	"github.com/FelineStateMachine/012/internal/numfmt"
)

// Now is the clock used by TODAY() and NOW(), and for the year of dates
// typed without one; tests replace it.
var Now = time.Now

// Typed dates and times become serial day numbers, as in Sheets; see
// numfmt's calendar.

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
	return numfmt.TimeSerial(h, m, sec), f, true
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
	for i, n := range numfmt.MonthNames {
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
	if m < 1 || m > 12 || d < 1 || d > numfmt.DaysIn(y, m) {
		return 0, Format{}, false
	}
	return numfmt.DateSerial(y, m, d), f, true
}
