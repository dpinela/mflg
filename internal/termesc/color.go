package termesc

import "strconv"

type GraphicFlag int

// Constants for non-color graphic attributes.
const (
	StyleNone GraphicFlag = 0
	StyleBold GraphicFlag = 1
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

type GraphicAttribute interface {
	forEachSGRCode(func(int))
}

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
