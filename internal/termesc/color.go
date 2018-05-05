package termesc

import "strconv"

// A GraphicFlag is a graphic attribute that can be defined by a single number in an ANSI escape code.
type GraphicFlag int

// Constants for non-color graphic attributes.
const (
	StyleNone     GraphicFlag = 0
	StyleBold     GraphicFlag = 1
	StyleInverted GraphicFlag = 7
	ColorDefault  GraphicFlag = 39
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

// Color24 is a 24-bit RGB color, with 8 bits per channel.
type Color24 struct {
	R, G, B uint8
}

func (c Color24) forEachSGRCode(f func(int)) {
	f(38)
	f(2)
	f(int(c.R))
	f(int(c.G))
	f(int(c.B))
}

func (c GraphicFlag) forEachSGRCode(f func(int)) { f(int(c)) }

// A GraphicAttribute is any graphic attribute that can be define by ANSI escape codes.
// Currently it can only be a GraphicFlag or Color24; Color8 will be implemented in the future.
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
