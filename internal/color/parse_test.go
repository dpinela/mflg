package color

import (
	"testing"
)

var badColors = []string{"EFCA39", "#89ACB", "#", "", "#GG8000", "xtup"}

var goodColors = []struct {
	in  string
	out Color
}{
	{"#ABCDEF", Color{0xAB, 0xCD, 0xEF}},
	{"#8950BE", Color{0x89, 0x50, 0xBE}},
	{"#000000", Color{}},
	{"#FFFFFF", Color{255, 255, 255}},
}

func TestBadColors(t *testing.T) {
	for _, s := range badColors {
		if c, err := Parse(s); err == nil {
			t.Errorf("Parse(%q) = %+v; want error", s, c)
		}
	}
}

func TestGoodColors(t *testing.T) {
	for _, tt := range goodColors {
		if c, err := Parse(tt.in); err != nil {
			t.Errorf("Parse(%q) got error, want %+v", tt.in, tt.out)
		} else if c != tt.out {
			t.Errorf("Parse(%q) = %+v, want %+v", tt.in, c, tt.out)
		}
	}
}
