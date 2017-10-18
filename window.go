package main

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/streak"
	"github.com/dpinela/mflg/internal/termesc"
	"golang.org/x/text/unicode/norm"
)

type point struct {
	x, y int
}

type textRange struct {
	start, end point
}

type window struct {
	w             io.Writer
	width, height int
	topLine       int   //The index of the topmost line being displayed
	cursorPos     point //The cursor position relative to the top left corner of the window

	window2TextY []int //A mapping from window y-coordinates to text y-coordinates

	moveTicker streak.Tracker

	dirty bool //Indicates whether the contents of the window's buffer have been modified
	//Indicates whether the visible part of the window has changed since it was last
	//drawn
	needsRedraw bool

	buf      *buffer.Buffer // The buffer being edited in the window
	searchRE *regexp.Regexp // The regexp currently in use for search and replace ops
}

func newWindow(console io.Writer, width, height int, buf *buffer.Buffer) *window {
	return &window{
		w: console, width: width, height: height,
		buf: buf, needsRedraw: true, moveTicker: streak.Tracker{Interval: time.Second / 5},
	}
}

// resize sets the window's height and width, then updates the layout
// and cursor position accordingly.
func (w *window) resize(newHeight, newWidth int) {
	gw := w.gutterWidth()
	if w.cursorPos.x+gw >= newWidth {
		w.cursorPos.x = newWidth - gw - 1
	}
	if w.cursorPos.y >= newHeight {
		w.cursorPos.y = newHeight - 1
	}
	w.width = newWidth
	w.height = newHeight
	w.needsRedraw = true
}

// Returns the length of line, as visually seen on the console.
func displayLen(line []byte) int {
	n := 0
	for i := 0; i < len(line); {
		p := norm.NFC.NextBoundary(line, true)
		if p == 1 && line[0] == '\n' {
			break
		} else {
			n += displayLenChar(line[:p])
		}
		line = line[p:]
	}
	return n
}

func ndigits(x int) int {
	if x == 0 {
		return 1
	}
	n := 0
	for x > 0 {
		x /= 10
		n++
	}
	return n
}

// This is here mainly so tests don't break when we introduce configurable
// tab widths.
func (w *window) tabWidth() int { return 4 }

func (w *window) gutterWidth() int {
	return ndigits(w.buf.LineCount()) + 1
}

func (w *window) textAreaWidth() int {
	return w.width - w.gutterWidth() - 1
}

// redraw updates the screen to reflect the logical window contents.
// If shouldDraw is false, it only updates the layout.
func (w *window) redraw(shouldDraw bool) error {
	if !w.needsRedraw {
		return nil
	}
	if shouldDraw {
		if _, err := fmt.Fprint(w.w, termesc.SetCursorPos(1, 1), termesc.ClearScreen); err != nil {
			return err
		}
	}
	w.window2TextY = w.window2TextY[:0]
	// We leave one space at the right end of the window so that we can always type
	// at the end of lines
	lineWidth := w.textAreaWidth()
	var rest []byte
	for ty, wy := w.topLine, 0; ty < w.buf.LineCount() && wy < w.height; ty++ {
		line := w.buf.Line(ty)
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		line = bytes.Replace(line, tab, fourSpaces, -1)
		for wy < w.height {
			line, rest = wrapLine(line, lineWidth)
			ender := crlf
			if wy+1 >= w.height {
				ender = nil
			}
			if shouldDraw {
				if _, err := fmt.Fprintf(w.w, "%*d %s%s", w.gutterWidth()-1, ty+1, line, ender); err != nil {
					return err
				}
			}
			w.window2TextY = append(w.window2TextY, ty)
			wy++
			if len(rest) == 0 {
				break
			}
			line = rest
		}
		if len(rest) > 0 {
			w.window2TextY = append(w.window2TextY, ty)
		}
	}
	if len(rest) == 0 {
		p := &w.window2TextY
		*p = append(*p, (*p)[len(*p)-1]+1)
	}
	w.roundCursorPos()
	// Keep an extra entry in the table so that we can convert positions one line past the bottom of the window
	// We don't need the converse at the top end because right now the line past
	// the top is always the previous line

		/*tp := w.windowCoordsToTextCoords(w.cursorPos)
		fmt.Fprintf(w.w, "\r\x1B[1mw: %v t: %v\x1B[0m", w.cursorPos, tp)*/
	w.needsRedraw = !shouldDraw
	return nil
}

func wrapLine(line []byte, width int) (first, rest []byte) {
	x := 0
	i := 0
	for i < len(line) {
		i += norm.NFC.NextBoundary(line[i:], true)
		x++
		if x == width {
			return line[:i], line[i:]
		}
	}
	return line, nil
}

// updateMoveSpeed updates the arrow key streak count and returns the corresponding
// cursor movement speed.
func (w *window) updateMoveSpeed() int {
	const (
		accelThreshold = 6
		accelMoveSpeed = 5
	)
	if w.moveTicker.Tick() >= accelThreshold {
		return accelMoveSpeed
	}
	return 1
}

func (w *window) repeatMove(move func()) {
	n := w.updateMoveSpeed()
	for i := 0; i < n; i++ {
		move()
	}
}

func (w *window) moveCursorDown() {
	if w.window2TextY[w.cursorPos.y+1] >= w.buf.LineCount() {
		return
	}
	if w.cursorPos.y < w.height-1 {
		w.cursorPos.y++
		w.roundCursorPos()
	} else {
		w.topLine++
		w.needsRedraw = true
		w.redraw(false)
	}
}

func (w *window) moveCursorUp() {
	switch {
	case w.cursorPos.y > 0:
		w.cursorPos.y--
		w.roundCursorPos()
	case w.topLine > 0:
		w.topLine--
		w.needsRedraw = true
		w.redraw(false)
	}
}

func (w *window) gotoLine(y int) {
	w.topLine = y
	if w.topLine >= w.buf.LineCount() {
		w.topLine = w.buf.LineCount() - 1
	}
	w.cursorPos.y = 0
	w.needsRedraw = true
	w.redraw(false)
}

func (w *window) roundCursorPos() {
	w.cursorPos = w.textCoordsToWindowCoords(w.windowCoordsToTextCoords(w.cursorPos))
}

func (w *window) moveCursorLeft() {
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	if tp.x > 0 {
		w.cursorPos = w.textCoordsToWindowCoords(point{y: tp.y, x: tp.x - 1})
	} else if tp.y > 0 {
		w.moveCursorUp()
		w.cursorPos.x = w.textAreaWidth() - 1
		w.roundCursorPos()
	}
	/*if w.cursorX > 0 {
		w.cursorX--
	} else if w.cursorY > 0 || w.topLine > 0 {*/

}

func (w *window) moveCursorRight() {
	oldWp := w.cursorPos
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	w.cursorPos = w.textCoordsToWindowCoords(point{y: tp.y, x: tp.x + 1})
	if w.cursorPos == oldWp && tp.y + 1 < w.buf.LineCount() {
		w.cursorPos.x = 0
		w.moveCursorDown()
	}
	if w.cursorPos.x >= w.width {
		w.cursorPos.x = w.width
	}
}

func (w *window) searchRegexp(re *regexp.Regexp) {
	w.searchRE = re
	for i, line := range w.buf.SliceLines(0, w.buf.LineCount()) {
		if re.Match(line) {
			w.gotoLine(i)
			return
		}
	}
}

func displayLenChar(char []byte) int {
	if len(char) == 1 && char[0] == '\t' {
		return 4
	}
	return 1
}

// Window coordinates: a (y, x) position within the window.
// Text coordinates: a (line, column) position within the text.

func (w *window) scanLineUntil(line []byte, stopAt func(wx, wy, tx int) bool) (wx, wy, tx int) {
	lineWidth := w.textAreaWidth()
	for len(line) != 0 && !stopAt(wx, wy, tx) {
		// Allow (y, 0) to map to the first character of a wrapped line's continuation
		/*if wx == lineWidth && stopAt(0, wy + 1, tx) {
			return 0, wy + 1, tx
		}*/
		p := norm.NFC.NextBoundary(line, true)
		// Don't count the final newline if there is one
		if p == 1 && line[0] == '\n' {
			break
		}
		wx += displayLenChar(line[:p])
		for wx >= lineWidth+1 {
			wy++
			wx -= lineWidth
		}
		tx++
		line = line[p:]
	}
	return
}

func (w *window) windowCoordsToTextCoords(wp point) (tp point) {
	if wp.y >= len(w.window2TextY)-1 {
		wp.y = len(w.window2TextY) - 1
	}
	ty := w.window2TextY[wp.y]
	if ty >= w.buf.LineCount() {
		ty = w.buf.LineCount() - 1
	}
	baseWY := w.lineStartY(ty)
	line := w.buf.Line(ty)
	_, _, tx := w.scanLineUntil(line, func(x, y, _ int) bool {
		return x >= wp.x && baseWY+y >= wp.y
	})
	return point{y: ty, x: tx}
}

func (w *window) lineStartY(ty int) (wy int) {
	for wy, y := range w.window2TextY {
		if y == ty {
			return wy
		}
	}
	return 0
}

func (w *window) textCoordsToWindowCoords(tp point) (wp point) {
	line := w.buf.Line(tp.y)
	wx, wy, _ := w.scanLineUntil(line, func(_, _, i int) bool { return i >= tp.x })
	return point{y: w.lineStartY(tp.y) + wy, x: wx}
}

func prefixUntil(text []byte, pred func(rune) bool) []byte {
	if p := bytes.IndexFunc(text, pred); p != -1 {
		return text[:p]
	}
	return text
}

func (w *window) typeText(text []byte) {
	w.dirty = true
	w.needsRedraw = true
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	switch text[0] {
	case '\r':
		indent := prefixUntil(w.buf.Line(tp.y), func(c rune) bool { return !(c == '\t' || c == ' ') })
		w.buf.InsertLineBreak(tp.y, tp.x)
		w.buf.Insert(indent, tp.y+1, 0)
		w.moveCursorDown() // Needed to ensure scrolling if necessary
		w.cursorPos = w.textCoordsToWindowCoords(point{len(indent), tp.y+1})
	default:
		w.buf.Insert(text, tp.y, tp.x)
		w.moveCursorRight()
		/*n := displayLen(text)
		for i := 0; i < n; i++ {
			w.moveCursorRight()
		}*/

	}
}

func (w *window) backspace() {
	w.dirty = true
	w.needsRedraw = true
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	newX := 0
	if tp.y > 0 {
		newX = displayLen(w.buf.Line(tp.y - 1))
	}
	w.buf.DeleteChar(tp.y, tp.x)
	if w.cursorPos.x == 0 {
		w.moveCursorUp()
		w.cursorPos.x = newX
		w.roundCursorPos()
	} else {
		w.cursorPos = w.textCoordsToWindowCoords(point{y: tp.y, x: tp.x - 1})
	}
}

func (w *window) handleMouseEvent(ev termesc.MouseEvent) {
	switch ev.Button {
	case termesc.ReleaseButton:
		w.cursorPos.x = ev.X - w.gutterWidth()
		w.cursorPos.y = ev.Y
		w.roundCursorPos()
	case termesc.ScrollUpButton:
		if w.topLine > 0 {
			w.topLine--
			w.needsRedraw = true
		}
	case termesc.ScrollDownButton:
		if w.topLine < w.buf.LineCount()-1 {
			w.topLine++
			w.needsRedraw = true
		}
	}
}

func (w *window) printAtBottom(text string) error {
	_, err := fmt.Fprintf(w.w, "%s%s%s", termesc.SetCursorPos(2000, 1), termesc.ClearLine, text)
	return err
}
