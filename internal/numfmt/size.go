package numfmt

import (
	"math"
	"strings"

	"github.com/FelineStateMachine/012/internal/locale"
)

// sizeUnits are the units of the Size format, a thousand times apart,
// as nushell shows file sizes.
var sizeUnits = []string{"B", "kB", "MB", "GB", "TB", "PB", "EB"}

// Size shows v bytes as nushell shows a file size: whole bytes below
// 1000 ("512 B"), and above that dec decimals of the largest unit, a
// thousand times apart, that keeps the number under 1000. Like nushell,
// it cuts the digits past dec rather than rounding them, so 2998 bytes
// is "2.9 kB", never a unit it hasn't reached. Negative sizes show a
// minus sign.
func Size(v float64, dec int) string { return SizeIn(v, dec, locale.Canonical) }

// SizeIn is Size as shown in loc: 1,6 kB in de-DE.
func SizeIn(v float64, dec int, loc *locale.Locale) string {
	sign := ""
	if v < 0 {
		sign, v = "-", -v
	}
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return sign + "∞ B"
	}
	unit := 0
	for unit < len(sizeUnits)-1 && v >= 1000 {
		v /= 1000
		unit++
	}
	pat := "0"
	if unit > 0 && dec > 0 {
		pat += "." + strings.Repeat("0", dec)
	} else {
		dec = 0
	}
	return sign + FormatIn(truncTo(v, dec), pat, loc) + " " + sizeUnits[unit]
}

// truncTo cuts v to dec decimals. The nudge keeps a value that is
// exactly on a digit, such as 1.6 held as 1.5999999, from losing it.
func truncTo(v float64, dec int) float64 {
	p := math.Pow10(dec)
	return math.Trunc(v*p*(1+1e-12)) / p
}
