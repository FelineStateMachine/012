package sheet

import (
	"fmt"
	"math"
	"math/rand/v2"
	"strconv"
	"strings"
	"testing"
)

// printedDec is numDec found only by printing v, as for inputs
// plainDec leaves.
func printedDec(v float64, input string) (uint8, bool) {
	if strconv.FormatFloat(v, 'f', -1, 64) == input {
		return 0, true
	}
	i := strings.IndexByte(input, '.')
	if i < 0 || len(input)-i-1 >= maxDec {
		return 0, false
	}
	d := len(input) - i - 1
	if strconv.FormatFloat(v, 'f', d, 64) == input {
		return uint8(d + 1), true
	}
	return 0, false
}

// Where plainDec answers without printing, it answers as printing does.
func TestPlainDecAsPrinted(t *testing.T) {
	inputs := []string{"0", "-0", "0.0", "-0.0", "0.50", "1.5", "100", "100.0", "12.30", "007", "0.1", "0.000", "1e5", ".5", "5.",
		"123456789012345", "1234567890123456", "0.000000000000000000001", "99999999999999.9", "-42.4200", "1,200", "$5", "1.2.3", "--1", "+1"}
	r := rand.New(rand.NewPCG(3, 4))
	for range 20000 {
		switch r.IntN(3) {
		case 0:
			inputs = append(inputs, fmt.Sprintf("%.*f", r.IntN(8), (r.Float64()-0.5)*math.Pow(10, float64(r.IntN(18)))))
		case 1:
			inputs = append(inputs, strconv.FormatInt(r.Int64N(1e17)-5e16, 10))
		default:
			inputs = append(inputs, strconv.FormatFloat(r.NormFloat64()*1e6, 'f', -1, 64))
		}
	}
	answered := 0
	for _, in := range inputs {
		v, err := strconv.ParseFloat(in, 64)
		if err != nil {
			continue
		}
		d, ok := plainDec(v, in)
		if !ok {
			continue
		}
		answered++
		if pd, pok := printedDec(v, in); !pok || pd != d {
			t.Errorf("%q: plainDec %d, printed %d %v", in, d, pd, pok)
		}
	}
	if answered < len(inputs)/5 {
		t.Errorf("plainDec answered %d of %d", answered, len(inputs))
	}
}
