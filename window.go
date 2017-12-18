package main

import (
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/clipboard"
	"github.com/dpinela/mflg/internal/streak"
	"github.com/dpinela/mflg/internal/termesc"

	"github.com/mattn/go-runewidth"
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
	width, height int
	// options for representing the start position of a window within a row:
	// - full text point: resizes do not move the relative scroll position within the document, but needs recomputing that text point when scrolling up or down. Also may result in text shifting horizontally when scrolling vertically after a resize.
	// - wrapped line index (relative to current text line): meaning changes whenever the window is resized, which means that scroll position changes too
	// - wrapped line index (global): same issue, but even worse.
	//
	// Which invariants do we want to preserve when scrolling?
	// - Scrolling vertically must only move text vertically (makes sense)
	// - Scrolling should always move all lines up or down (precludes fully pinning the text point)
	//
	// What if we allow the start position to drift a bit when necessary on resizes?
	// Let (X, Y) be the start position.
	// When the text between (0, Y) and (X, Y) would occupy less space than the new window width, advance the start position
	// just enough to ensure that it does. Exception: if X=0, do nothing (the start is at a line boundary).
	topLine   int   //The index (in window space) of the topmost line being displayed
	cursorPos point //The cursor position in window space

	selectionAnchor      optionalPoint // The last point marked as an initial selection bound by keyboard
	mouseSelectionAnchor optionalPoint // Same, but using the mouse
	selection            optionalTextRange

	customGutterText string // If not empty, this text is displayed in each gutter line instead of the line number

	moveTicker streak.Tracker

	dirty bool //Indicates whether the contents of the window's buffer have been modified
	//Indicates whether the visible part of the window has changed since it was last
	//drawn
	needsRedraw bool

	buf        *buffer.Buffer        // The buffer being edited in the window
	wrappedBuf *buffer.WrappedBuffer // Wrapped version of buf, for display purposes
	tabString  string                // The string that should be inserted when typing a tab

	searchRE *regexp.Regexp // The regexp currently in use for search and replace ops
}

type optionalPoint struct {
	point
	Set bool
}

func (op *optionalPoint) Put(p point) {
	op.point = p
	op.Set = true
}

type optionalTextRange struct {
	textRange
	Set bool
}

func (otr *optionalTextRange) Put(tr textRange) {
	otr.textRange = tr
	otr.Set = true
}

func newWindow(width, height int, buf *buffer.Buffer) *window {
	w := &window{
		width: width, height: height,
		buf:         buf,
		tabString:   tabString(buf.IndentType()),
		needsRedraw: true, moveTicker: streak.Tracker{Interval: time.Second / 5},
	}
	// We leave one space at the right end of the window so that we can always type
	// at the end of lines
	w.wrappedBuf = buffer.NewWrapped(buf, w.textAreaWidth(), displayLenChar)
	return w
}

func tabString(width int) string {
	if width == buffer.IndentTabs {
		return "\t"
	}
	return strings.Repeat(" ", width)
}

func (w *window) viewportCursorPos() point { return point{w.cursorPos.x, w.cursorPos.y - w.topLine} }
func (w *window) cursorInViewport() bool {
	return w.cursorPos.y >= w.topLine && w.cursorPos.y < w.topLine+w.height
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
	w.wrappedBuf.SetWidth(w.textAreaWidth())
	w.needsRedraw = true
}

// Returns the length of line, as visually seen on the console.
func displayLen(line string) int {
	n := 0
	for i := 0; i < len(line); {
		p := buffer.NextCharBoundary(line)
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
func (w *window) tabWidth() int {
	if w.tabString == "\t" {
		return 4
	}
	return len(w.tabString)
}

func (w *window) gutterWidth() int {
	if w.customGutterText != "" {
		return runewidth.StringWidth(w.customGutterText) + 1
	}
	return ndigits(w.buf.LineCount()) + 1
}

func (w *window) textAreaWidth() int {
	return w.width - w.gutterWidth() - 1
}

func (w *window) redraw(console io.Writer) error { return w.redrawAtYOffset(console, 0) }

// redrawAtYOffset renders the window's contents onto a console.
// If the console is nil, it only updates the window's layout.
func (w *window) redrawAtYOffset(console io.Writer, yOffset int) error {
	if !w.needsRedraw {
		return nil
	}
	if console != nil {
		if _, err := fmt.Fprint(console, termesc.SetCursorPos(yOffset+1, 1), termesc.ClearScreenForward); err != nil {
			return err
		}
	}
	lines := w.wrappedBuf.Lines(w.topLine, w.topLine+w.height)
	tf := textFormatter{src: lines,
		invertedRegion: w.selection, gutterWidth: w.gutterWidth(), gutterText: w.customGutterText}
	for wy := 0; wy < w.height; wy++ {
		line, ok := tf.formatNextLine(wy+1 >= w.height)
		if !ok {
			break
		}
		if console != nil {
			if _, err := console.Write(line); err != nil {
				return err
			}
		}
	}
	w.roundCursorPos()

	/*tp := w.windowCoordsToTextCoords(w.cursorPos)
	fmt.Fprintf(w.w, "\r\x1B[1mw: %v t: %v\x1B[0m", w.cursorPos, tp)*/
	w.needsRedraw = console == nil
	return nil
}

type textFormatter struct {
	src            []buffer.WrappedLine
	invertedRegion optionalTextRange
	gutterText     string
	gutterWidth    int

	line int
	buf  []byte
}

const tabWidth = 4

func (tf *textFormatter) formatNextLine(last bool) ([]byte, bool) {
	if tf.line >= len(tf.src) {
		return nil, false
	}
	line := trimNewline(tf.src[tf.line].Text)
	tp := tf.src[tf.line].Start
	if tf.gutterText != "" {
		tf.buf = append(tf.buf[:0], tf.gutterText...)
	} else {
		tf.buf = strconv.AppendInt(tf.buf[:0], int64(tp.Y)+1, 10)
	}
	for i := len(tf.buf); i < tf.gutterWidth; i++ {
		tf.buf = append(tf.buf, ' ')
	}
	if tf.invertedRegion.Set && tp.Y > tf.invertedRegion.begin.y && tp.Y <= tf.invertedRegion.end.y {
		tf.buf = append(tf.buf, termesc.ReverseVideo...)
	}
	for len(line) > 0 {
		if tf.invertedRegion.Set {
			switch (point{tp.X, tp.Y}) {
			case tf.invertedRegion.begin:
				tf.buf = append(tf.buf, termesc.ReverseVideo...)
			case tf.invertedRegion.end:
				tf.buf = append(tf.buf, termesc.ResetGraphicAttributes...)
			}
		}
		n := buffer.NextCharBoundary(line)
		if line[:n] == "\t" {
			tf.appendSpaces(tabWidth)
		} else if line[:n] != "\n" {
			tf.buf = append(tf.buf, line[:n]...)
		}
		line = line[n:]
		tp.X++
	}
	if tf.invertedRegion.Set && ((tp.Y >= tf.invertedRegion.begin.y && tp.Y < tf.invertedRegion.end.y) || tf.invertedRegion.end == point{tp.X, tp.Y}) {
		tf.buf = append(tf.buf, termesc.ResetGraphicAttributes...)
	}
	if !last {
		tf.buf = append(tf.buf, '\r', '\n')
	}
	tf.line++
	return tf.buf, true
}

func (tf *textFormatter) appendSpaces(n int) {
	for i := 0; i < n; i++ {
		tf.buf = append(tf.buf, ' ')
	}
}

func trimNewline(line string) string {
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

func (w *window) canMoveCursorDown() bool { return w.cursorPos.y < w.topLine+w.height-1 }
func (w *window) canMoveCursorUp() bool   { return w.cursorPos.y > w.topLine }

func (w *window) moveCursorDown() {
	if !w.canMoveCursorDown() {
		w.scrollDown()
	}
	if w.canMoveCursorDown() {
		w.cursorPos.y++
		w.roundCursorPos()
	}
}

func (w *window) moveCursorUp() {
	if !w.canMoveCursorUp() {
		w.scrollUp()
	}
	if w.canMoveCursorUp() {
		w.cursorPos.y--
		w.roundCursorPos()
	}
}

func (w *window) scrollDown() {
	if w.wrappedBuf.HasLine(w.topLine + w.height) {
		w.topLine++
		w.needsRedraw = true
	}
}

func (w *window) scrollUp() {
	if w.topLine > 0 {
		w.topLine--
		w.needsRedraw = true
	}
}

func (w *window) gotoLine(y int) {
	if w.wrappedBuf.HasLine(y) {
		w.topLine = y
		w.needsRedraw = true
	}
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

func (w *window) moveCursorRight() { w.moveCursorRightBy(1) }

// moveCursorRightBy moves the cursor n characters to the right, moving to the start of the next line if the
// current line isn't long enough for that.
func (w *window) moveCursorRightBy(n int) {
	oldWp := w.cursorPos
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	w.cursorPos = w.textCoordsToWindowCoords(point{y: tp.y, x: tp.x + n})
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
		if re.MatchString(line) {
			w.gotoLine(i)
			return
		}
	}
}

func displayLenChar(char string) int {
	if char == "\t" {
		return 4
	}
	return runewidth.StringWidth(char)
}

func scanLineUntil(line string, startTx int, stopAt func(wx, tx int) bool) (wx, tx int) {
	tx = startTx
	for len(line) != 0 && !stopAt(wx, tx) {
		p := buffer.NextCharBoundary(line)
		if line[:p] == "\n" {
			break
		}
		wx += displayLenChar(line[:p])
		tx++
		line = line[p:]
	}
	return
}

func (w *window) windowCoordsToTextCoords(wp point) (tp point) {
	line := w.wrappedBuf.Line(wp.y)
	_, tx := scanLineUntil(line.Text, line.Start.X, func(wx, _ int) bool { return wx >= wp.x })
	return point{tx, line.Start.Y}
}

func (w *window) textCoordsToWindowCoords(tp point) (wp point) {
	wy := w.wrappedBuf.WindowYForTextPos(buffer.Point{tp.x, tp.y})
	line := w.wrappedBuf.Line(wy)
	wx, _ := scanLineUntil(line.Text, line.Start.X, func(_, tx int) bool { return tx >= tp.x })
	return point{wx, wy}
}

func prefixUntil(text string, pred func(rune) bool) string {
	if p := strings.IndexFunc(text, pred); p != -1 {
		return text[:p]
	}
	return text
}

func leadingIndentation(text string) string {
	return prefixUntil(text, func(c rune) bool { return !(c == '\t' || c == ' ') })
}

func (w *window) typeText(text string) {
	if w.selection.Set {
		w.backspace()
	}
	w.dirty = true
	w.needsRedraw = true
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	switch text[0] {
	case '\r':
		indent := leadingIndentation(w.buf.Line(tp.y))
		w.wrappedBuf.InsertLineBreak(tp.y, tp.x)
		w.wrappedBuf.Insert(indent, tp.y+1, 0)
		w.moveCursorDown() // Needed to ensure scrolling if necessary
		w.cursorPos = w.textCoordsToWindowCoords(point{len(indent), tp.y + 1})
	case '\t':
		w.wrappedBuf.Insert(w.tabString, tp.y, tp.x)
		w.moveCursorRightBy(len(w.tabString))
	default:
		w.wrappedBuf.Insert(text, tp.y, tp.x)
		w.moveCursorRight()
	}
}

// visibleTextEnd returns the text-space coordinates of the first character that lies below the viewport.
func (w *window) visibleTextEnd() point {
	// If there's offscreen lines, the start of the first line below the bottom of the viewport is exactly
	// the point we want.
	line := w.wrappedBuf.Line(w.topLine + w.height)
	if w.wrappedBuf.HasLine(w.topLine + w.height) {
		return point{line.Start.X, line.Start.Y}
	}
	// If there are no offscreen lines, then we got the last line in the whole buffer; scan it to the end
	// to find the end point.
	_, tx := scanLineUntil(line.Text, line.Start.X, func(_, _ int) bool { return false })
	return point{tx, line.Start.Y}
}

func (w *window) isTextPointOnscreen(tp point) bool {
	return tp.y >= w.topLine && !w.visibleTextEnd().Less(tp)
}

func (w *window) backspace() {
	w.dirty = true
	w.needsRedraw = true
	if w.selection.Set {
		w.wrappedBuf.DeleteRange(w.selection.begin.y, w.selection.begin.x, w.selection.end.y, w.selection.end.x)
		w.gotoTextPos(w.selection.begin)
		w.selection = optionalTextRange{}
		return
	}
	newX := 0
	if w.cursorPos.y > 0 {
		newX = displayLen(w.wrappedBuf.Line(w.cursorPos.y - 1).Text)
	}
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	w.wrappedBuf.DeleteChar(tp.y, tp.x)
	switch {
	case tp.x > 0:
		w.gotoTextPos(point{y: tp.y, x: tp.x - 1})
	case tp.y > 0:
		w.moveCursorUp()
		w.cursorPos.x = newX
		w.roundCursorPos()
	}
}

func (w *window) gotoTextPos(tp point) {
	if !w.isTextPointOnscreen(tp) {
		w.gotoLine(tp.y)
	}
	w.cursorPos = w.textCoordsToWindowCoords(tp)
}

func (w *window) markSelectionBound() {
	// A window may be in one of three states of a cycle, regarding selection:
	// 0. No selection, no point marked (the initial state)
	// 1. One bound marked
	// 2. Two bounds marked (selection complete)
	// Each call to this method advances the cycle by one step.
	if w.selectionAnchor.Set {
		w.selectToCursorPos(&w.selectionAnchor)
	} else {
		w.clearSelection()
		w.selectionAnchor.Put(w.windowCoordsToTextCoords(w.cursorPos))
	}
}

func (w *window) selectToCursorPos(anchor *optionalPoint) {
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	// Prevent empty selections (and if using the mouse, also clear the selection when clicking)
	if anchor.Set && tp == anchor.point {
		w.clearSelection()
		*anchor = optionalPoint{}
		return
	}
	if tp.Less(anchor.point) {
		tp, anchor.point = anchor.point, tp
	}
	w.selection.Put(textRange{anchor.point, tp})
	*anchor = optionalPoint{}
	w.needsRedraw = true
}

// resetSelectionState deselects whatever text is currently selected and also removes any bounds marked.
// In other words, it puts the window back in state 0 of the selection cycle.
func (w *window) resetSelectionState() {
	w.clearSelection()
	w.selectionAnchor = optionalPoint{}
	w.mouseSelectionAnchor = optionalPoint{}
}

func (w *window) clearSelection() {
	if w.selection.Set {
		w.needsRedraw = true
	}
	w.selection = optionalTextRange{}
}

func (w *window) copySelection() {
	if w.selection.Set {
		go clipboard.Copy(w.buf.CopyRange(w.selection.begin.y, w.selection.begin.x, w.selection.end.y, w.selection.end.x))
	}
}

func (w *window) paste() {
	data, err := clipboard.Paste()
	if err != nil || len(data) == 0 {
		return
	}
	if w.selection.Set {
		w.backspace()
	}
	s := string(data)
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	w.wrappedBuf.Insert(s, tp.y, tp.x)
	w.gotoTextPos(posAfterInsertion(tp, s))
	w.needsRedraw = true
}

func posAfterInsertion(tp point, data string) point {
	for len(data) > 0 {
		n := buffer.NextCharBoundary(data)
		if data[:n] == "\n" {
			tp.y++
			tp.x = 0
		} else {
			tp.x++
		}
		data = data[n:]
	}
	return tp
}

func (w *window) handleMouseEvent(ev termesc.MouseEvent) {
	switch ev.Button {
	case termesc.LeftButton:
		w.setCursorPosFromMouse(ev)
		w.mouseSelectionAnchor.Put(w.windowCoordsToTextCoords(w.cursorPos))
	case termesc.ReleaseButton:
		w.setCursorPosFromMouse(ev)
		if w.mouseSelectionAnchor.Set {
			w.selectToCursorPos(&w.mouseSelectionAnchor)
		}
	case termesc.ScrollUpButton:
		w.scrollUp()
		w.roundCursorPos()
	case termesc.ScrollDownButton:
		w.scrollDown()
		w.roundCursorPos()
	}
}

func (w *window) inMouseSelection() bool {
	return w.mouseSelectionAnchor.Set
}

func (w *window) setCursorPosFromMouse(ev termesc.MouseEvent) {
	w.cursorPos.x = ev.X - w.gutterWidth()
	w.cursorPos.y = ev.Y + w.topLine
	w.roundCursorPos()
}
