package numfmt

import (
	"testing"

	"github.com/FelineStateMachine/012/internal/locale"
)

func TestSize(t *testing.T) {
	for _, tc := range []struct {
		v    float64
		dec  int
		want string
	}{
		{0, 1, "0 B"},
		{512, 1, "512 B"},
		{999, 1, "999 B"},
		{999.6, 1, "1.0 kB"},
		{1000, 1, "1.0 kB"},
		{1646, 1, "1.6 kB"},
		{1646, 0, "2 kB"},
		{1646, 2, "1.65 kB"},
		{999_999, 1, "1.0 MB"},
		{2_000_000, 1, "2.0 MB"},
		{1.5e9, 1, "1.5 GB"},
		{1.2e12, 1, "1.2 TB"},
		{8e18, 1, "8.0 EB"},
		{1e21, 1, "1000.0 EB"},
		{-5, 1, "-5 B"},
		{-1500, 1, "-1.5 kB"},
	} {
		if got := Size(tc.v, tc.dec); got != tc.want {
			t.Errorf("Size(%v, %d) = %q, want %q", tc.v, tc.dec, got, tc.want)
		}
	}
	de, _ := locale.Lookup("de-DE")
	if got := SizeIn(1646, 1, de); got != "1,6 kB" {
		t.Errorf("de-DE: %q", got)
	}
}
