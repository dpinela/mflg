package termesc

import "strconv"

// A GraphicFlag is a graphic attribute that can be defined by a single number in an ANSI escape code.
type GraphicFlag int

// Constants for non-color graphic attributes.
const (
	StyleNone     GraphicFlag = 0
	StyleBold     GraphicFlag = 1
	StyleInverted GraphicFlag = 7
)

// Constants for the 3-bit ANSI color palette.
const (
	ColorBlack GraphicFlag = 30 + iota
	ColorRed
	ColorGreen
	ColorYellow
	ColorBlue
	ColorMagenta
	ColorCyan
	ColorWhite
)

func (c GraphicFlag) forEachSGRCode(f func(int)) { f(int(c)) }

// A GraphicAttribute is any graphic attribute that can be define by ANSI escape codes.
// Currently it can only be a GraphicFlag; Color8 and Color24 will be implemented in the future.
type GraphicAttribute interface {
	// Yields the numbers to put in the CSI ... ; ... m sequence for this attribute.
	forEachSGRCode(func(int))
}

// SetGraphicAttributes returns a code that applies the specified graphic attributes to all future text written
// to the terminal, in the order given.
func SetGraphicAttributes(attrs ...GraphicAttribute) string {
	b := make([]byte, len(csi), 64)
	copy(b, csi)
	for _, attr := range attrs {
		attr.forEachSGRCode(func(x int) {
			if len(b) > len(csi) {
				b = append(b, ';')
			}
			b = strconv.AppendInt(b, int64(x), 10)
		})
	}
	return string(append(b, 'm'))
}
