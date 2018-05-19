package termesc

import (
	"os"
	"strconv"

	"github.com/dpinela/mflg/internal/color"
)

var hasTruecolor = os.Getenv("COLORTERM") == "truecolor"

// A GraphicFlag is a graphic attribute that can be defined by a single number in an ANSI escape code.
type GraphicFlag int

// Constants for non-color graphic attributes.
const (
	StyleNone              GraphicFlag = 0
	StyleBold              GraphicFlag = 1
	StyleNotBold           GraphicFlag = 22
	StyleUnderline         GraphicFlag = 4
	StyleNotUnderline      GraphicFlag = 24
	StyleInverted          GraphicFlag = 7
	StyleNotInverted       GraphicFlag = 27
	ColorDefault           GraphicFlag = 39
	ColorDefaultBackground GraphicFlag = 49
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

// OutputColor returns the input color, reducing the color depth if the terminal does
// not support full 24-bit color codes.
func OutputColor(c color.Color) GraphicAttribute { return outputColor(c, false) }

// OutputColorBackground is like OutputColor, but returns a code that sets the background
// color instead.
func OutputColorBackground(c color.Color) GraphicAttribute { return outputColor(c, true) }

func outputColor(c color.Color, bg bool) GraphicAttribute {
	if hasTruecolor {
		return color24{c.R, c.G, c.B, bg}
	}
	nr := narrowChannel(c.R)
	ng := narrowChannel(c.G)
	nb := narrowChannel(c.B)
	return color8{16 + 36*nr + 6*ng + nb, bg}
}

func narrowChannel(x uint8) int {
	return int(x) * 5 / 255
}

// color24 is a 24-bit RGB color, with 8 bits per channel.
type color24 struct {
	R, G, B    uint8
	Background bool
}

func (c color24) forEachSGRCode(f func(int)) {
	if c.Background {
		f(48)
	} else {
		f(38)
	}
	f(2)
	f(int(c.R))
	f(int(c.G))
	f(int(c.B))
}

type color8 struct {
	Color      int
	Background bool
}

func (c color8) forEachSGRCode(f func(int)) {
	if c.Background {
		f(48)
	} else {
		f(38)
	}
	f(5)
	f(int(c.Color))
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
