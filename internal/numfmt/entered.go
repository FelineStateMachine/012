package numfmt

// Entered keeps the first 15 significant digits of a number written in
// decimal (digits, a point, an exponent) and makes the digits after them
// zeros, as Sheets and Excel do with a number typed into a formula:
// =123456789012345678 is 123456789012345000. Text with no more than 15
// digits comes back as it is. Numbers typed into cells keep every digit,
// as files and imports write them with 17 to round-trip.
func Entered(s string) string {
	sig := 0
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == 'e' || c == 'E':
			return s
		case c < '0' || c > '9':
		case sig > 0 || c != '0':
			sig++
			if sig > 15 {
				return zeroDigits(s, i)
			}
		}
	}
	return s
}

// zeroDigits makes the digits of s from i up to any exponent zeros.
func zeroDigits(s string, i int) string {
	b := []byte(s)
	for ; i < len(b) && b[i] != 'e' && b[i] != 'E'; i++ {
		if b[i] >= '1' && b[i] <= '9' {
			b[i] = '0'
		}
	}
	return string(b)
}
