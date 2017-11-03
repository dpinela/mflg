package main

import (
	"bytes"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/clipboard"
	"github.com/dpinela/mflg/internal/streak"
	"github.com/dpinela/mflg/internal/termesc"
	"golang.org/x/text/unicode/norm"
)

type point struct {
	x, y int
}

// Less returns true if p is lexicographically ordered before q,
// considering the y-coordinate first.
func (p point) Less(q point) bool {
	if p.y < q.y {
		return true
	}
	if p.y > q.y {
		return false
	}
	return p.x < q.x
}

type textRange struct {
	begin, end point
}

type window struct {
	w             io.Writer
	width, height int
	topLine       int   //The index of the topmost line being displayed
	cursorPos     point //The cursor position relative to the top left corner of the window

	selectionAnchor      *point // The last point marked as an initial selection bound by keyboard
	mouseSelectionAnchor *point // Same, but using the mouse
	selection            *textRange

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
	tf := textFormatter{tp: point{0, w.topLine}, src: w.buf, lineWidth: w.textAreaWidth(),
		invertedRegion: w.selection}
	for wy := 0; wy < w.height; wy++ {
		ty := tf.tp.y
		line, ok := tf.formatNextLine()
		if !ok {
			break
		}
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
	}
	if len(tf.curLine) > 0 {
		w.window2TextY = append(w.window2TextY, tf.tp.y)
	} else {
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

type textFormatter struct {
	tp             point
	src            *buffer.Buffer
	lineWidth      int
	invertedRegion *textRange
	curLine, buf   []byte
	spacesCarry    int
}

const tabWidth = 4

func (tf *textFormatter) formatNextLine() ([]byte, bool) {
	if len(tf.curLine) == 0 {
		if tf.tp.y >= tf.src.LineCount() {
			return nil, false
		}
		tf.curLine = trimNewline(tf.src.Line(tf.tp.y))
	}
	totalW := tf.spacesCarry
	tf.buf = tf.buf[:0]
	if tf.invertedRegion != nil && tf.tp.y > tf.invertedRegion.begin.y && tf.tp.y <= tf.invertedRegion.end.y {
		tf.buf = append(tf.buf, termesc.ReverseVideo...)
	}
	tf.appendSpaces(tf.spacesCarry)
	tf.spacesCarry = 0
	for len(tf.curLine) > 0 {
		if tf.invertedRegion != nil {
			switch tf.tp {
			case tf.invertedRegion.begin:
				tf.buf = append(tf.buf, termesc.ReverseVideo...)
			case tf.invertedRegion.end:
				tf.buf = append(tf.buf, termesc.ResetGraphicAttributes...)
			}
		}
		n := norm.NFC.NextBoundary(tf.curLine, true)
		if n == 1 && tf.curLine[0] == '\t' {
			w := min(tf.lineWidth-totalW, tabWidth)
			totalW += w
			tf.appendSpaces(w)
			tf.spacesCarry = tabWidth - w
		} else if !(n == 1 && tf.curLine[0] == '\n') {
			tf.buf = append(tf.buf, tf.curLine[:n]...)
			totalW++
		}
		tf.curLine = tf.curLine[n:]
		tf.tp.x++
		if totalW == tf.lineWidth {
			break
		}
	}
	if tf.invertedRegion != nil && ((tf.tp.y >= tf.invertedRegion.begin.y && tf.tp.y < tf.invertedRegion.end.y) || tf.invertedRegion.end == tf.tp) {
		tf.buf = append(tf.buf, termesc.ResetGraphicAttributes...)
	}
	if len(tf.curLine) == 0 {
		tf.tp.y++
		tf.tp.x = 0
	}
	return tf.buf, true
}

func (tf *textFormatter) appendSpaces(n int) {
	for i := 0; i < n; i++ {
		tf.buf = append(tf.buf, ' ')
	}
}

func trimNewline(line []byte) []byte {
	if len(line) > 0 && line[len(line)-1] == '\n' {
		return line[:len(line)-1]
	}
	return line
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
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

}

func (w *window) moveCursorRight() {
	oldWp := w.cursorPos
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	w.cursorPos = w.textCoordsToWindowCoords(point{y: tp.y, x: tp.x + 1})
	if w.cursorPos == oldWp && tp.y+1 < w.buf.LineCount() {
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
	if w.selection != nil {
		w.backspace()
	}
	w.dirty = true
	w.needsRedraw = true
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	switch text[0] {
	case '\r':
		indent := prefixUntil(w.buf.Line(tp.y), func(c rune) bool { return !(c == '\t' || c == ' ') })
		w.buf.InsertLineBreak(tp.y, tp.x)
		w.buf.Insert(indent, tp.y+1, 0)
		w.moveCursorDown() // Needed to ensure scrolling if necessary
		w.cursorPos = w.textCoordsToWindowCoords(point{len(indent), tp.y + 1})
	default:
		w.buf.Insert(text, tp.y, tp.x)
		w.moveCursorRight()
	}
}

func (w *window) visibleTextEnd() point {
	lastLineWY := len(w.window2TextY) - 2
	ty := w.window2TextY[lastLineWY]
	_, _, tx := w.scanLineUntil(w.buf.Line(ty), func(_, y, _ int) bool { return lastLineWY+y >= w.height })
	return point{tx, ty}
}

func (w *window) isTextPointOnscreen(tp point) bool {
	return tp.y >= w.topLine && !w.visibleTextEnd().Less(tp)
}

func (w *window) backspace() {
	w.dirty = true
	w.needsRedraw = true
	if w.selection != nil {
		w.buf.DeleteRange(w.selection.begin.y, w.selection.begin.x, w.selection.end.y, w.selection.end.x)
		if !w.isTextPointOnscreen(w.selection.begin) {
			w.gotoLine(w.selection.begin.y)
		}
		w.cursorPos = w.textCoordsToWindowCoords(w.selection.begin)
		w.selection = nil
		return
	}
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

func (w *window) markSelectionBound() {
	// A window may be in one of three states of a cycle, regarding selection:
	// 0. No selection, no point marked (the initial state)
	// 1. One bound marked
	// 2. Two bounds marked (selection complete)
	// Each call to this method advances the cycle by one step.
	if w.selectionAnchor != nil {
		w.selectToCursorPos(&w.selectionAnchor)
	} else {
		w.clearSelection()
		tp := w.windowCoordsToTextCoords(w.cursorPos)
		w.selectionAnchor = &tp
	}
}

func (w *window) selectToCursorPos(anchor **point) {
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	// Prevent empty selections (and if using the mouse, also clear the selection when clicking)
	if tp == **anchor {
		w.clearSelection()
		*anchor = nil
		return
	}
	if tp.Less(**anchor) {
		tp, **anchor = **anchor, tp
	}
	w.selection = &textRange{**anchor, tp}
	*anchor = nil
	w.needsRedraw = true
}

// resetSelectionState deselects whatever text is currently selected and also removes any bounds marked.
// In other words, it puts the window back in state 0 of the selection cycle.
func (w *window) resetSelectionState() {
	w.clearSelection()
	w.selectionAnchor = nil
	w.mouseSelectionAnchor = nil
}

func (w *window) clearSelection() {
	if w.selection != nil {
		w.needsRedraw = true
	}
	w.selection = nil
}

func (w *window) copySelection() {
	if w.selection == nil {
		return
	}
	go clipboard.Copy(w.buf.CopyRange(w.selection.begin.y, w.selection.begin.x, w.selection.end.y, w.selection.end.x))
}

func (w *window) paste() {
	data, err := clipboard.Paste()
	if err != nil || len(data) == 0 {
		return
	}
	if w.selection != nil {
		w.backspace()
	}
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	w.buf.Insert(data, tp.y, tp.x)
	w.needsRedraw = true
}

func (w *window) handleMouseEvent(ev termesc.MouseEvent) {
	switch ev.Button {
	case termesc.LeftButton:
		w.setCursorPosFromMouse(ev)
		tp := w.windowCoordsToTextCoords(w.cursorPos)
		w.mouseSelectionAnchor = &tp
	case termesc.ReleaseButton:
		w.setCursorPosFromMouse(ev)
		if w.mouseSelectionAnchor != nil {
			w.selectToCursorPos(&w.mouseSelectionAnchor)
		}
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

func (w *window) inMouseSelection() bool {
	return w.mouseSelectionAnchor != nil
}

func (w *window) setCursorPosFromMouse(ev termesc.MouseEvent) {
	w.cursorPos.x = ev.X - w.gutterWidth()
	w.cursorPos.y = ev.Y
	w.roundCursorPos()
}

func (w *window) printAtBottom(text string) error {
	_, err := fmt.Fprintf(w.w, "%s%s%s", termesc.SetCursorPos(2000, 1), termesc.ClearLine, text)
	return err
}
