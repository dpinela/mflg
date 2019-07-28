package termdraw

import (
	"fmt"
	"io"

	"github.com/mattn/go-runewidth"

	"github.com/dpinela/mflg/internal/color"
	"github.com/dpinela/mflg/internal/termesc"
)

// A Style describes the appearance of a chunk of text.
//
// The zero Style means non-bold, non-underline text with the default colors
// for the output device.
type Style struct {
	Foreground, Background  *color.Color
	Bold, Italic, Underline bool
	Inverted                bool
}

// A Cell represents a single character along with the style it should be displayed with.
// The zero Cell acts as an empty space.
type Cell struct {
	Content string
	Style   Style
}

// Screen represents a buffered terminal screen.
//
// Changes made to the contents by Resize, Put, Clear, SetCursorPos, SetCursorVisible or SetTitle
// are not reflected on the terminal until Flip is called.
type Screen struct {
	console io.Writer
	width   int

	current           []Cell
	cursorPos         Point
	cursorVisible     bool
	prevCursorVisible bool
	title             string

	titleNeedsRedraw bool
}

// Point is a point in the coordinate system of the terminal, in which X increases from left to right
// and Y from top to bottom.
// The zero Point, (0, 0), represents the top-left corner of the screen.
type Point struct {
	X, Y int
}

// NewScreen creates a new, blank Screen connected to a terminal with the given dimensions.
func NewScreen(out io.Writer, size Point) *Screen {
	return &Screen{console: out, width: size.X, current: make([]Cell, size.X*size.Y)}
}

// Size returns the current dimensions of the Screen.
func (s *Screen) Size() Point { return Point{X: s.width, Y: len(s.current) / s.width} }

// Resize updates the dimensions of the Screen, then clears it.
func (s *Screen) Resize(size Point) {
	s.width = size.X
	n := size.X * size.Y
	if n < cap(s.current) {
		s.current = s.current[:n]
		s.Clear()
	} else {
		s.current = make([]Cell, n)
	}
}

// Clear sets all cells in the Screen to blank spaces.
func (s *Screen) Clear() {
	for i := range s.current {
		s.current[i] = Cell{}
	}
}

// Put replaces the content of the cell at position p.
func (s *Screen) Put(p Point, c Cell) { s.current[p.Y*s.width+p.X] = c }

// SetTitle sets the terminal's title.
func (s *Screen) SetTitle(t string) {
	s.title = t
	s.titleNeedsRedraw = true
}

// SetCursorPos sets the cursor position.
func (s *Screen) SetCursorPos(p Point) { s.cursorPos = p }

// SetCursorVisible sets whether the cursor is visible.
func (s *Screen) SetCursorVisible(visible bool) { s.cursorVisible = visible }

var styleReset = termesc.SetGraphicAttributes(termesc.StyleNone)

// Flip replaces the contents of the screen with the current contents of the Screen's buffer.
// It also updates the title and cursor, if necessary.
//
// It assumes that nothing else has written to the terminal since the last call to Flip, unless
// this is the first such call for that Screen.
func (s *Screen) Flip() error {
	if s.titleNeedsRedraw {
		if _, err := s.console.Write([]byte(termesc.SetTitle(s.title))); err != nil {
			return err
		}
		s.titleNeedsRedraw = false
	}
	if _, err := fmt.Fprint(s.console, termesc.SetCursorPos(1, 1), termesc.ClearScreenForward); err != nil {
		return err
	}
	curStyle := Style{}
	var buf []byte
	for i := 0; i < len(s.current); i += s.width {
		buf = buf[:0]
		row := trimTrailingBlanks(s.current[i : i+s.width])
		for x := 0; x < len(row); x++ {
			c := row[x]
			if c.Style != curStyle {
				buf = append(buf, styleReset...)
				buf = append(buf, makeSGRString(&c.Style)...)
				curStyle = c.Style
			}
			if c.Content == "" {
				c.Content = " "
			}
			if runewidth.StringWidth(c.Content) > 1 {
				x++ // skip next cell
			}
			buf = append(buf, c.Content...)
		}
		buf = append(buf, styleReset...)
		if i+s.width < len(s.current) {
			buf = append(buf, '\r', '\n')
		}
		if _, err := s.console.Write(buf); err != nil {
			return err
		}
	}
	if s.cursorVisible != s.prevCursorVisible {
		code := termesc.HideCursor
		if s.cursorVisible {
			code = termesc.ShowCursor
		}
		if _, err := s.console.Write([]byte(code)); err != nil {
			return err
		}
	}
	s.prevCursorVisible = s.cursorVisible
	if s.cursorVisible {
		if _, err := s.console.Write([]byte(termesc.SetCursorPos(s.cursorPos.Y+1, s.cursorPos.X+1))); err != nil {
			return err
		}
	}
	return nil
}

func trimTrailingBlanks(cs []Cell) []Cell {
	for i := len(cs) - 1; i >= 0; i-- {
		if cs[i].Content != "" {
			return cs[:i+1]
		}
	}
	return cs[:0]
}

func makeSGRString(s *Style) string {
	var params []termesc.GraphicAttribute
	// At the end of each highlighted region, these flags are all reset,
	// so at the start of this one we know that they're all off.
	if fg := s.Foreground; fg != nil {
		params = append(params, termesc.OutputColor(*fg))
	}
	if bg := s.Background; bg != nil {
		params = append(params, termesc.OutputColorBackground(*bg))
	}
	if s.Bold {
		params = append(params, termesc.StyleBold)
	}
	if s.Italic {
		params = append(params, termesc.StyleItalic)
	}
	if s.Underline {
		params = append(params, termesc.StyleUnderline)
	}
	if s.Inverted {
		params = append(params, termesc.StyleInverted)
	}
	return termesc.SetGraphicAttributes(params...)
}
