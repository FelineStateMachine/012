package sheet

import (
	"math"
	"strings"

	"github.com/FelineStateMachine/012/internal/numfmt"
)

var (
	dateFormat     = returns(Preset(FmtDate))
	timeFormat     = returns(Preset(FmtTime))
	dateTimeFormat = returns(Preset(FmtDateTime))
)

func init() {
	define(
		&FuncDef{Name: "DATE", Args: "year, month, day", Desc: "A date from its parts; months and days past the end roll over", Min: 3, Max: 3,
			eval: numeric(func(x []float64) Value {
				y, m, d := math.Trunc(x[0]), math.Trunc(x[1]), math.Trunc(x[2])
				if y >= 0 && y < 1900 {
					y += 1900 // DATE(26, 1, 1) is 1926, as in Sheets
				}
				if y < 0 || y >= 10000 || math.Abs(m) > 1e6 || math.Abs(d) > 1e8 {
					return ErrNum
				}
				v := numfmt.DateSerial(int(y), int(m), int(d))
				if v < 0 {
					return ErrNum
				}
				return num(v)
			}), format: dateFormat},
		&FuncDef{Name: "TIME", Args: "hour, minute, second", Desc: "A time of day from its parts", Min: 3, Max: 3,
			eval: numeric(func(x []float64) Value {
				secs := math.Trunc(x[0])*3600 + math.Trunc(x[1])*60 + math.Trunc(x[2])
				if secs < 0 {
					return ErrNum
				}
				return num(math.Mod(secs, 86400) / 86400)
			}), format: timeFormat},
		&FuncDef{Name: "TODAY", Desc: "Today's date, updated on every change", Max: 0, Volatile: true,
			eval: func([]Node, lookup) Value { return num(math.Floor(numfmt.SerialOf(Now()))) }, format: dateFormat},
		&FuncDef{Name: "NOW", Desc: "The current date and time, updated on every change", Max: 0, Volatile: true,
			eval: func([]Node, lookup) Value { return num(numfmt.SerialOf(Now())) }, format: dateTimeFormat},
		&FuncDef{Name: "YEAR", Args: "date", Desc: "Year of a date", Min: 1, Max: 1,
			eval: datePart(func(y, _, _ int) int { return y })},
		&FuncDef{Name: "MONTH", Args: "date", Desc: "Month of a date, 1 to 12", Min: 1, Max: 1,
			eval: datePart(func(_, m, _ int) int { return m })},
		&FuncDef{Name: "DAY", Args: "date", Desc: "Day of the month of a date", Min: 1, Max: 1,
			eval: datePart(func(_, _, d int) int { return d })},
		&FuncDef{Name: "WEEKDAY", Args: "date, [type]", Desc: "Day of the week: 1 is Sunday, or Monday with type 2", Min: 1, Max: 2,
			eval: func(args []Node, get lookup) Value {
				d, err := dateArg(args[0], get)
				if err != nil {
					return *err
				}
				typ, err := intArg(args, 1, 1, get)
				if err != nil {
					return *err
				}
				days, _ := numfmt.SplitSerial(d)
				wd := numfmt.Weekday(days) // 0 = Sunday
				switch typ {
				case 1:
					return num(float64(wd + 1))
				case 2:
					return num(float64((wd+6)%7 + 1))
				case 3:
					return num(float64((wd + 6) % 7))
				}
				return ErrNum
			}},
		&FuncDef{Name: "HOUR", Args: "time", Desc: "Hour of a time, 0 to 23", Min: 1, Max: 1,
			eval: timePart(func(s int) int { return s / 3600 })},
		&FuncDef{Name: "MINUTE", Args: "time", Desc: "Minute of a time, 0 to 59", Min: 1, Max: 1,
			eval: timePart(func(s int) int { return s / 60 % 60 })},
		&FuncDef{Name: "SECOND", Args: "time", Desc: "Second of a time, 0 to 59", Min: 1, Max: 1,
			eval: timePart(func(s int) int { return s % 60 })},
		&FuncDef{Name: "EDATE", Args: "start_date, months", Desc: "The same day a number of months away", Min: 2, Max: 2,
			eval: monthShift(false), format: dateFormat},
		&FuncDef{Name: "EOMONTH", Args: "start_date, months", Desc: "The last day of the month a number of months away", Min: 2, Max: 2,
			eval: monthShift(true), format: dateFormat},
		&FuncDef{Name: "DATEDIF", Args: `start_date, end_date, unit`, Desc: `Time between dates in "Y", "M", "D", "MD", "YM" or "YD"`, Min: 3, Max: 3,
			eval: datedif},
		&FuncDef{Name: "DAYS", Args: "end_date, start_date", Desc: "Number of days between two dates", Min: 2, Max: 2,
			eval: func(args []Node, get lookup) Value {
				end, err := dateArg(args[0], get)
				if err != nil {
					return *err
				}
				start, err := dateArg(args[1], get)
				if err != nil {
					return *err
				}
				return num(math.Floor(end) - math.Floor(start))
			}},
		&FuncDef{Name: "NETWORKDAYS", Args: "start_date, end_date, [holidays]", Desc: "Number of weekdays between two dates, counting both", Min: 2, Max: 3,
			eval: networkdays},
		&FuncDef{Name: "DATEVALUE", Args: "date_string", Desc: "The date a text such as \"2026-09-26\" stands for", Min: 1, Max: 1,
			eval: func(args []Node, get lookup) Value {
				v, ok := parsedText(args[0], get)
				if ok != nil {
					return *ok
				}
				d, f, parsed := parseDateTime(v)
				if !parsed || f.Kind == FmtTime || f.Kind == FmtDuration {
					return ErrValue
				}
				return num(math.Floor(d))
			}, format: dateFormat},
		&FuncDef{Name: "TIMEVALUE", Args: "time_string", Desc: "The time of day a text such as \"2:30 PM\" stands for", Min: 1, Max: 1,
			eval: func(args []Node, get lookup) Value {
				v, ok := parsedText(args[0], get)
				if ok != nil {
					return *ok
				}
				if d, _, parsed := parseDateTime(v); parsed {
					return num(d - math.Floor(d))
				}
				return ErrValue
			}, format: timeFormat},
	)
}

// parsedText requires a text argument, as DATEVALUE and TIMEVALUE do.
func parsedText(n Node, get lookup) (string, *Value) {
	v := eval(n, get)
	switch v.Kind {
	case Error:
		return "", errOf(v)
	case Text:
		return v.Str, nil
	}
	return "", &ErrValue
}

// dateArg is a date argument: a serial number or text such as
// "2026-09-26". Negative serials are #NUM!.
func dateArg(n Node, get lookup) (float64, *Value) {
	d, err := numArg(n, get)
	if err != nil {
		return 0, err
	}
	if d < 0 {
		return 0, &ErrNum
	}
	return d, nil
}

func datePart(part func(y, m, d int) int) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		d, err := dateArg(args[0], get)
		if err != nil {
			return *err
		}
		days, _ := numfmt.SplitSerial(d)
		return num(float64(part(numfmt.Civil(days))))
	}
}

func timePart(part func(secs int) int) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		d, err := dateArg(args[0], get)
		if err != nil {
			return *err
		}
		_, secs := numfmt.SplitSerial(d)
		return num(float64(part(secs)))
	}
}

// monthShift builds EDATE and EOMONTH. EDATE keeps the day, pulling it
// back to the month's last day when it doesn't exist (Jan 31 + 1 month is
// Feb 28).
func monthShift(endOfMonth bool) func([]Node, lookup) Value {
	return func(args []Node, get lookup) Value {
		d, err := dateArg(args[0], get)
		if err != nil {
			return *err
		}
		months, err := intArg(args, 1, 0, get)
		if err != nil {
			return *err
		}
		y, m, day := numfmt.Civil(int64(math.Floor(d)))
		y, m = numfmt.AddMonths(y, m, months)
		if y < 1 || y > 9999 {
			return ErrNum
		}
		last := numfmt.DaysIn(y, m)
		if endOfMonth || day > last {
			day = last
		}
		v := numfmt.DateSerial(y, m, day)
		if v < 0 {
			return ErrNum
		}
		return num(v)
	}
}

func datedif(args []Node, get lookup) Value {
	start, err := dateArg(args[0], get)
	if err != nil {
		return *err
	}
	end, err := dateArg(args[1], get)
	if err != nil {
		return *err
	}
	unit, err := textArg(args[2], get)
	if err != nil {
		return *err
	}
	s, e := int64(math.Floor(start)), int64(math.Floor(end))
	if s > e {
		return ErrNum
	}
	sy, sm, sd := numfmt.Civil(s)
	ey, em, ed := numfmt.Civil(e)
	months := (ey-sy)*12 + em - sm
	if ed < sd {
		months--
	}
	switch strings.ToUpper(unit) {
	case "Y":
		return num(float64(months / 12))
	case "M":
		return num(float64(months))
	case "D":
		return num(float64(e - s))
	case "YM":
		return num(float64(months % 12))
	case "MD":
		if ed >= sd {
			return num(float64(ed - sd))
		}
		// Days from sd in the month before the end date.
		py, pm := ey, em-1
		if pm == 0 {
			py, pm = ey-1, 12
		}
		return num(float64(e) - numfmt.DateSerial(py, pm, sd))
	case "YD":
		// Days since the last anniversary of the start date.
		y := ey
		if em < sm || em == sm && ed < sd {
			y--
		}
		anniv := numfmt.DateSerial(y, sm, min(sd, numfmt.DaysIn(y, sm)))
		return num(float64(e) - anniv)
	}
	return ErrNum
}

func networkdays(args []Node, get lookup) Value {
	start, err := dateArg(args[0], get)
	if err != nil {
		return *err
	}
	end, err := dateArg(args[1], get)
	if err != nil {
		return *err
	}
	holidays := map[int64]bool{}
	if len(args) > 2 {
		hs, err := nums(args[2:], get)
		if err != nil {
			return *err
		}
		for _, h := range hs {
			holidays[int64(math.Floor(h))] = true
		}
	}
	s, e := int64(math.Floor(start)), int64(math.Floor(end))
	sign := 1.0
	if s > e {
		s, e, sign = e, s, -1
	}
	if e-s > 1e7 {
		return ErrNum
	}
	n := 0
	for d := s; d <= e; d++ {
		if wd := numfmt.Weekday(d); wd != 0 && wd != 6 && !holidays[d] {
			n++
		}
	}
	return num(sign * float64(n))
}
