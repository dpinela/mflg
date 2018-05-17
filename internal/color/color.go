package color

import (
	"fmt"
	"github.com/pkg/errors"
	"strconv"
)

// A Color is a 8-bit-per-channel RGB color.
type Color struct {
	R, G, B uint8
}

// Parse returns the RGB values corresponding to the color described by s.
// The string may be a CSS-style hex code (#ABCDEF).
func Parse(s string) (Color, error) {
	if !(len(s) == 7 && s[0] == '#') {
		return Color{}, fmt.Errorf("color: parse %q: not a valid hex string", s)
	}
	n, err := strconv.ParseInt(s[1:], 16, 32)
	if err != nil {
		return Color{}, errors.WithMessage(err, fmt.Sprintf("color: parse %q", s))
	}
	return Color{uint8(n >> 16), uint8(n >> 8), uint8(n)}, nil
}
