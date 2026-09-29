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
// thousand times apart, that keeps the number under 1000 once rounded
// ("1.6 kB", "1.0 MB"). Negative sizes show a minus sign.
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
	for unit < len(sizeUnits)-1 && roundTo(v, 0) >= 1000 {
		v /= 1000
		unit++
	}
	if unit > 0 && roundTo(v, dec) >= 1000 && unit < len(sizeUnits)-1 {
		v /= 1000
		unit++
	}
	pat := "0"
	if unit > 0 && dec > 0 {
		pat += "." + strings.Repeat("0", dec)
	}
	return sign + FormatIn(v, pat, loc) + " " + sizeUnits[unit]
}

// roundTo rounds v to dec decimals, halves away from zero.
func roundTo(v float64, dec int) float64 {
	p := math.Pow10(dec)
	return math.Round(v*p) / p
}
