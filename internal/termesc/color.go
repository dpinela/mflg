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

func (c GraphicFlag) appendNumbers(xs []int) []int { return append(xs, int(c)) }

type GraphicAttribute interface {
	appendNumbers([]int) []int
}

func SetGraphicAttributes(attrs ...GraphicAttribute) string {
	xs := make([]int, 0, 8)
	b := []byte(csi)
	for _, attr := range attrs {
		for _, x := range attr.appendNumbers(xs[:0]) {
			if len(b) > len(csi) {
				b = append(b, ';')
			}
			b = strconv.AppendInt(b, int64(x), 10)
		}
	}
	return string(append(b, 'm'))
}
