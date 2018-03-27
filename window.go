package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/clipboard"
	"github.com/dpinela/mflg/internal/config"
	"github.com/dpinela/mflg/internal/streak"
	"github.com/dpinela/mflg/internal/termesc"

	"github.com/mattn/go-runewidth"
)

type point = buffer.Point
type textRange = buffer.Range

type window struct {
	width, height int
	topLine       int   //The index (in window space) of the topmost line being displayed
	cursorPos     point //The cursor position in window space

	selectionAnchor      optionalPoint // The last point marked as an initial selection bound by keyboard
	mouseSelectionAnchor optionalPoint // Same, but using the mouse
	wordSelectionAnchor  optionalTextRange
	selection            optionalTextRange

	// If not empty, this text is displayed in each gutter line instead of the line number.
	// This shouldn't be set directly, as it affects the gutter width and therefore the wrapping in the main text area:
	// use setGutterText instead.
	customGutterText string

	moveTicker streak.Tracker

	lastMouseRelease, lastMouseLeftPress timedMouseEvent

	onChange         func() // If not nil, called whenever the window's buffer is modified
	readonly         bool
	modificationTime time.Time // The time when the last edit occurred
	undoStack        []snapshot

	needsRedraw bool // Indicates whether the visible part of the window has changed since it was last drawn

	buf        *buffer.Buffer        // The buffer being edited in the window
	wrappedBuf *buffer.WrappedBuffer // Wrapped version of buf, for display purposes
	tabString  string                // The string that should be inserted when typing a tab
	tabWidth   int                   // The width with which hard tabs are displayed
	langConfig config.LangConfig

	app *application // The application that owns this window
}

// The maximum time between two changes such that they will be undone together.
// It is a variable so that it can be changed for testing.
var changeCoalescingInterval = time.Second

type snapshot struct {
	content   *buffer.Buffer
	selection optionalTextRange
	cursorPos point
}

type timedMouseEvent struct {
	termesc.MouseEvent
	when   time.Time
	isDrag bool
}

func (tev *timedMouseEvent) put(ev termesc.MouseEvent) {
	tev.MouseEvent = ev
	tev.when = time.Now()
	tev.isDrag = false
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

func newWindow(width, height int, buf *buffer.Buffer, tabWidth int) *window {
	w := &window{
		width: width, height: height,
		buf:       buf,
		tabString: tabString(buf.IndentType()), tabWidth: tabWidth,
		needsRedraw: true, moveTicker: streak.Tracker{Interval: time.Second / 5},
	}
	// We leave one space at the right end of the window so that we can always type
	// at the end of lines
	w.wrappedBuf = buffer.NewWrapped(buf, w.textAreaWidth(), w.displayLenChar)
	return w
}

func tabString(width int) string {
	if width == buffer.IndentTabs {
		return "\t"
	}
	return strings.Repeat(" ", width)
}

func (w *window) viewportCursorPos() point { return point{w.cursorPos.X, w.cursorPos.Y - w.topLine} }
func (w *window) windowPosInViewport(wp point) bool {
	return wp.Y >= w.topLine && wp.Y < w.topLine+w.height
}
func (w *window) textPosInViewport(tp point) bool {
	return w.windowPosInViewport(point{0, w.wrappedBuf.WindowYForTextPos(tp)})
}
func (w *window) cursorInViewport() bool { return w.windowPosInViewport(w.cursorPos) }

// resize sets the window's height and width, then updates the layout
// and cursor position accordingly.
func (w *window) resize(newHeight, newWidth int) {
	gw := w.gutterWidth()
	if w.cursorPos.X+gw >= newWidth {
		w.cursorPos.X = newWidth - gw - 1
	}
	w.width = newWidth
	w.height = newHeight
	w.updateWrapWidth()
	w.needsRedraw = true
}

// updateWrapWidth updates wrappedBuf's width to match the text area's current width.
// It should be called after any state change which causes that width to change, such as resizes or edits (the latter
// can cause the gutter width to change).
func (w *window) updateWrapWidth() { w.wrappedBuf.SetWidth(w.textAreaWidth()) }

func (w *window) setGutterText(text string) {
	w.customGutterText = text
	w.wrappedBuf.SetWidth(w.textAreaWidth())
}

func (w *window) formatBuffer() {
	if len(w.langConfig.Formatter) == 0 {
		return
	}
	w.readonly = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, w.langConfig.Formatter[0], w.langConfig.Formatter[1:]...)
		cmd.Stdin = w.buf.Reader()
		formattedText, err := cmd.Output()
		w.app.do(func() {
			w.readonly = false
			if err != nil {
				w.app.setNotification(err.Error())
				return
			}
			w.takeSnapshot()
			w.buf.ReadFrom(bytes.NewReader(formattedText))
			w.wrappedBuf.Reset(w.buf)
			w.notifyChange()
			w.needsRedraw = true
		})
	}()
}

// Returns the length of line, as visually seen on the console.
func (w *window) displayLen(line string) int {
	n := 0
	for i := 0; i < len(line); {
		p := buffer.NextCharBoundary(line)
		if p == 1 && line[0] == '\n' {
			break
		} else {
			n += w.displayLenChar(line[:p])
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
func (w *window) getTabWidth() int {
	if w.tabString == "\t" {
		return w.tabWidth
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
	if _, err := fmt.Fprint(console, termesc.SetCursorPos(yOffset+1, 1), termesc.ClearScreenForward); err != nil {
		return err
	}
	lines := w.wrappedBuf.Lines(w.topLine, w.topLine+w.height)
	tf := textFormatter{src: lines,
		invertedRegion: w.selection, gutterWidth: w.gutterWidth(), gutterText: w.customGutterText, tabWidth: w.getTabWidth()}
	for wy := 0; wy < w.height; wy++ {
		line, ok := tf.formatNextLine(wy+1 >= w.height)
		if !ok {
			break
		}
		if _, err := console.Write(line); err != nil {
			return err
		}
	}
	w.needsRedraw = console == nil
	return nil
}

type textFormatter struct {
	src            []buffer.WrappedLine
	invertedRegion optionalTextRange
	gutterText     string
	gutterWidth    int
	tabWidth       int

	line int
	buf  []byte
}

// Pre-compute the SGR escape sequences used in formatNextLine to avoid the expense of recomputing them repeatedly.
var (
	styleInverted     = termesc.SetGraphicAttributes(termesc.StyleInverted)
	styleResetToBold  = termesc.SetGraphicAttributes(termesc.StyleNone, termesc.StyleBold)
	styleResetToWhite = termesc.SetGraphicAttributes(termesc.StyleNone, termesc.ColorWhite)
	styleReset        = termesc.SetGraphicAttributes(termesc.StyleNone)
)

func (tf *textFormatter) formatNextLine(last bool) ([]byte, bool) {
	if tf.line >= len(tf.src) {
		return nil, false
	}
	line := strings.TrimSuffix(tf.src[tf.line].Text, "\n")
	tp := tf.src[tf.line].Start
	var gutterLen int
	if tf.gutterText != "" {
		tf.buf = append(tf.buf[:0], styleResetToBold...)
		gutterLen = runewidth.StringWidth(tf.gutterText)
		tf.buf = append(tf.buf, tf.gutterText...)
	} else {
		tf.buf = append(tf.buf[:0], styleResetToWhite...)
		n := len(tf.buf)
		tf.buf = strconv.AppendInt(tf.buf, int64(tp.Y)+1, 10)
		gutterLen = len(tf.buf) - n
	}
	tf.buf = append(tf.buf, styleReset...)
	for i := gutterLen; i < tf.gutterWidth; i++ {
		tf.buf = append(tf.buf, ' ')
	}
	if tf.invertedRegion.Set && !tp.Less(tf.invertedRegion.Begin) && tp.Less(tf.invertedRegion.End) {
		tf.buf = append(tf.buf, styleInverted...)
	}
	for len(line) > 0 {
		if tf.invertedRegion.Set {
			switch tp {
			case tf.invertedRegion.Begin:
				tf.buf = append(tf.buf, styleInverted...)
			case tf.invertedRegion.End:
				tf.buf = append(tf.buf, styleReset...)
			}
		}
		n := buffer.NextCharBoundary(line)
		if line[:n] == "\t" {
			tf.appendSpaces(tf.tabWidth)
		} else if line[:n] != "\n" {
			tf.buf = append(tf.buf, line[:n]...)
		}
		line = line[n:]
		tp.X++
	}
	if tf.invertedRegion.Set && ((tp.Y >= tf.invertedRegion.Begin.Y && tp.Y < tf.invertedRegion.End.Y) || tf.invertedRegion.End == tp) {
		tf.buf = append(tf.buf, styleReset...)
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

func (w *window) canMoveCursorDown() bool { return w.cursorPos.Y < w.topLine+w.height-1 }
func (w *window) canMoveCursorUp() bool   { return w.cursorPos.Y > w.topLine }

func (w *window) moveCursorDown() {
	if !w.canMoveCursorDown() {
		w.scrollDown()
	}
	if w.canMoveCursorDown() {
		w.cursorPos.Y++
		w.roundCursorPos()
	}
}

func (w *window) moveCursorUp() {
	if !w.canMoveCursorUp() {
		w.scrollUp()
	}
	if w.canMoveCursorUp() {
		w.cursorPos.Y--
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

func (w *window) gotoLine(ty int) {
	wy := w.wrappedBuf.WindowYForTextPos(buffer.Point{X: 0, Y: ty})
	if w.wrappedBuf.HasLine(wy) {
		w.topLine = wy
		w.cursorPos = point{X: 0, Y: wy}
		w.needsRedraw = true
	}
}

func (w *window) roundCursorPos() {
	w.cursorPos = w.textCoordsToWindowCoords(w.windowCoordsToTextCoords(w.cursorPos))
}

func (w *window) moveCursorLeft() {
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	if tp.X > 0 {
		w.cursorPos = w.textCoordsToWindowCoords(point{Y: tp.Y, X: tp.X - 1})
		w.followCursor()
	} else if tp.Y > 0 {
		w.moveCursorUp()
		w.cursorPos.X = w.textAreaWidth() - 1
		w.roundCursorPos()
	}
}

func (w *window) moveCursorRight() { w.moveCursorRightBy(1) }

// moveCursorRightBy moves the cursor n characters to the right, moving to the start of the next line if the
// current line isn't long enough for that.
func (w *window) moveCursorRightBy(n int) {
	oldWp := w.cursorPos
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	w.cursorPos = w.textCoordsToWindowCoords(point{Y: tp.Y, X: tp.X + n})
	// If we're at the end of a text-space line, we can move right no further within it; go to the next line.
	// Since this is the end of the line, it is guaranteed that the start of the next is at the start of the next
	// window-space line.
	if oldWp == w.cursorPos && tp.Y+1 < w.buf.LineCount() {
		w.cursorPos = point{0, w.cursorPos.Y + 1}
	}
	w.followCursor()
}

func (w *window) moveCursorLeftWord() {
	w.cursorPos = w.textCoordsToWindowCoords(w.buf.PrevWordBoundary(w.windowCoordsToTextCoords(w.cursorPos)))
	w.followCursor()
}

func (w *window) moveCursorRightWord() {
	w.cursorPos = w.textCoordsToWindowCoords(w.buf.NextWordBoundary(w.windowCoordsToTextCoords(w.cursorPos)))
	w.followCursor()
}

// followCursor scrolls the window towards the cursor, until the cursor is at the top or bottom edge.
// If the cursor is already in the viewport, does nothing.
func (w *window) followCursor() {
	switch {
	case w.cursorPos.Y < w.topLine:
		w.topLine = w.cursorPos.Y
		w.needsRedraw = true
	case w.cursorPos.Y >= w.topLine+w.height:
		w.topLine = w.cursorPos.Y - w.height + 1
		w.needsRedraw = true
	}
}

func (w *window) searchRegexp(re *regexp.Regexp, startY int) {
	for i, line := range w.buf.SliceLines(startY, w.buf.LineCount()) {
		if re.MatchString(strings.TrimSuffix(line, "\n")) {
			w.gotoLine(startY + i)
			return
		}
	}
	for i, line := range w.buf.SliceLines(0, startY) {
		if re.MatchString(strings.TrimSuffix(line, "\n")) {
			w.gotoLine(i)
			return
		}
	}
}

func separateSuffix(s, suffix string) (begin, foundSuffix string) {
	t := strings.TrimSuffix(s, suffix)
	if len(t) < len(s) {
		return t, suffix
	}
	return s, ""
}

// takeSnapshot puts a new snapshot on the undo stack if the last change occurred long enough ago.
// It should be called by each edit operation, before the edit actually takes place.
func (w *window) takeSnapshot() {
	now := time.Now()
	if now.Sub(w.modificationTime) > changeCoalescingInterval {
		w.undoStack = append(w.undoStack, snapshot{
			content:   w.buf.Copy(),
			selection: w.selection,
			cursorPos: w.cursorPos,
		})
	}
	w.modificationTime = now
}

func (w *window) notifyChange() {
	if w.onChange != nil {
		w.onChange()
	}
}

func (w *window) replaceRegexp(re *regexp.Regexp, replacement string) {
	if w.readonly {
		return
	}
	var lines []string
	// Process only the lines within the selection Y bounds.
	if w.selection.Set {
		lines = w.buf.SliceLines(w.selection.Begin.Y, w.selection.End.Y+1)
	} else {
		lines = w.buf.SliceLines(0, w.buf.LineCount())
	}
	changed := false
	for i, line := range lines {
		if w.selection.Set {
			i += w.selection.Begin.Y
		}
		// Do not replace text within the selection Y bounds, but outside the selection.
		begin := 0
		if w.selection.Set && i == w.selection.Begin.Y {
			begin = buffer.ByteIndexForChar(line, w.selection.Begin.X)
		}
		end := len(line)
		// Prevent the regexp from removing the newlines.
		// TODO: this should change if/when regexps can be applied across line boundaries.
		if strings.HasSuffix(line, "\n") {
			end--
		}
		if w.selection.Set && i == w.selection.End.Y {
			end = buffer.ByteIndexForChar(line, w.selection.End.X)
		}
		oldLine := line[begin:end]
		if newLine := re.ReplaceAllString(oldLine, replacement); newLine != oldLine {
			if !changed {
				w.takeSnapshot()
			}
			changed = true
			w.wrappedBuf.ReplaceLine(i, line[:begin]+newLine+line[end:])
			// We only need to adjust the selection in its final line, and then only at the bottom-right
			// end. The rest of it is guaranteed to stay in place, regardless of which replacements are
			// made, since newlines cannot be taken out and the replacement doesn't touch anything
			// before the beginning.
			if w.selection.Set && i == w.selection.End.Y {
				w.selection.End.X += buffer.CharCount(newLine) - buffer.CharCount(oldLine)
			}
			w.notifyChange()
			w.needsRedraw = true
		}
	}
}

func (w *window) displayLenChar(char string) int {
	if char == "\t" {
		return w.tabWidth
	}
	return runewidth.StringWidth(char)
}

func (w *window) scanLineUntil(line string, startTx int, stopAt func(wx, tx int) bool) (wx, tx int) {
	tx = startTx
	for len(line) != 0 && !stopAt(wx, tx) {
		p := buffer.NextCharBoundary(line)
		if line[:p] == "\n" {
			break
		}
		wx += w.displayLenChar(line[:p])
		tx++
		line = line[p:]
	}
	return
}

func (w *window) windowCoordsToTextCoords(wp point) (tp point) {
	line := w.wrappedBuf.Line(wp.Y)
	_, tx := w.scanLineUntil(line.Text, line.Start.X, func(wx, _ int) bool { return wx >= wp.X })
	return point{tx, line.Start.Y}
}

func (w *window) textCoordsToWindowCoords(tp point) (wp point) {
	wy := w.wrappedBuf.WindowYForTextPos(tp)
	line := w.wrappedBuf.Line(wy)
	wx, _ := w.scanLineUntil(line.Text, line.Start.X, func(_, tx int) bool { return tx >= tp.X })
	return point{X: wx, Y: wy}
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
	if w.readonly {
		return
	}
	if w.selection.Set {
		// This already takes a snapshot, since it's callable by itself.
		w.backspace()
	}
	w.takeSnapshot()
	w.needsRedraw = true
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	switch text[0] {
	case '\r':
		indent := leadingIndentation(w.buf.Line(tp.Y))
		w.wrappedBuf.InsertLineBreak(tp)
		w.wrappedBuf.Insert(indent, buffer.Point{Y: tp.Y + 1, X: 0})
		w.updateWrapWidth()
		w.moveCursorDown() // Needed to ensure scrolling if necessary
		w.cursorPos = w.textCoordsToWindowCoords(point{X: len(indent), Y: tp.Y + 1})
	case '\t':
		w.wrappedBuf.Insert(w.tabString, tp)
		w.updateWrapWidth()
		w.moveCursorRightBy(len(w.tabString))
	default:
		w.wrappedBuf.Insert(text, tp)
		w.updateWrapWidth()
		w.moveCursorRight()
	}
	w.notifyChange()
}

func (w *window) backspace() {
	if w.readonly {
		return
	}
	if w.selection.Set || w.cursorPos.X > 0 || w.cursorPos.Y > 0 {
		w.takeSnapshot()
		w.needsRedraw = true
	}
	if w.selection.Set {
		w.wrappedBuf.DeleteRange(w.selection.textRange)
		w.updateWrapWidth()
		w.gotoTextPos(w.selection.Begin)
		w.selection = optionalTextRange{}
		w.notifyChange()
		return
	}
	newX := 0
	if w.cursorPos.Y > 0 {
		newX = w.displayLen(w.wrappedBuf.Line(w.cursorPos.Y - 1).Text)
	}
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	w.wrappedBuf.DeleteChar(tp)
	w.notifyChange()
	w.updateWrapWidth()
	switch {
	case tp.X > 0:
		w.gotoTextPos(point{Y: tp.Y, X: tp.X - 1})
	case tp.Y > 0:
		w.moveCursorUp()
		w.cursorPos.X = newX
		w.roundCursorPos()
	}
}

func (w *window) gotoTextPos(tp point) {
	if !w.textPosInViewport(tp) {
		w.gotoLine(tp.Y)
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
	w.selection.Put(textRange{anchor.point, tp}.Normalize())
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
		go clipboard.Copy(w.buf.CopyRange(w.selection.textRange))
	}
}

func (w *window) cutSelection() {
	if w.selection.Set && !w.readonly {
		w.copySelection()
		w.backspace()
	}
}

func (w *window) paste() {
	if w.readonly {
		return
	}
	data, err := clipboard.Paste()
	if err != nil {
		return
	}
	w.insertText(data)
}

func (w *window) insertText(data []byte) {
	if w.readonly || len(data) == 0 {
		return
	}
	// backspace() already takes a snapshot, so in that case, we don't have to.
	if w.selection.Set {
		w.backspace()
	} else {
		w.takeSnapshot()
	}
	s := string(data)
	tp := w.windowCoordsToTextCoords(w.cursorPos)
	w.wrappedBuf.Insert(s, tp)
	w.updateWrapWidth()
	w.gotoTextPos(posAfterInsertion(tp, s))
	w.needsRedraw = true
	w.notifyChange()
}

func posAfterInsertion(tp point, data string) point {
	for len(data) > 0 {
		n := buffer.NextCharBoundary(data)
		if data[:n] == "\n" {
			tp.Y++
			tp.X = 0
		} else {
			tp.X++
		}
		data = data[n:]
	}
	return tp
}

func (w *window) undo()    { w.undoSince(len(w.undoStack) - 1) }
func (w *window) undoAll() { w.undoSince(0) }

// undoSince reverts all changes made since the i-th snapshot.
func (w *window) undoSince(i int) {
	if w.readonly || len(w.undoStack) == 0 {
		return
	}
	oldState := w.undoStack[i]
	w.undoStack = w.undoStack[:i]
	w.buf = oldState.content
	w.wrappedBuf.Reset(w.buf)
	w.selection = oldState.selection
	w.cursorPos = oldState.cursorPos
	w.notifyChange()
	w.needsRedraw = true
}

func (w *window) handleMouseEvent(ev termesc.MouseEvent) {
	const doubleClickInterval = time.Second / 2

	switch ev.Button {
	case termesc.LeftButton:
		if ev.Move {
			if w.lastMouseLeftPress.when.After(w.lastMouseRelease.when) {
				w.lastMouseLeftPress.isDrag = true
			}
			tp := w.textPosFromMouse(ev)
			w.cursorPos = w.textCoordsToWindowCoords(tp)
			// This is true if and only if a mouse selection has been started, but not ended yet;
			// during that period, update the selection as the cursor moves. Releasing is thus technically
			// a no-op, since when the release event fires we already detected the cursor moving into the
			// second end of the range.
			switch {
			case w.mouseSelectionAnchor.Set:
				w.selection.Put(textRange{w.mouseSelectionAnchor.point, tp}.Normalize())
				w.needsRedraw = true
			case w.wordSelectionAnchor.Set:
				newWord := w.buf.WordBoundsAt(tp)
				w.selection.Put(buffer.Range{
					minPoint(newWord.Begin, w.wordSelectionAnchor.Begin),
					maxPoint(newWord.End, w.wordSelectionAnchor.End)})
				w.needsRedraw = true
			}
		} else {
			tpNew := w.textPosFromMouse(ev)
			w.cursorPos = w.textCoordsToWindowCoords(tpNew)
			if time.Since(w.lastMouseLeftPress.when) < doubleClickInterval && !w.lastMouseLeftPress.isDrag {
				if w.trySelectWord(w.textPosFromMouse(w.lastMouseLeftPress.MouseEvent), tpNew) {
					w.wordSelectionAnchor.Put(w.selection.textRange)
				}
			} else {
				w.mouseSelectionAnchor.Put(tpNew)
			}
			w.lastMouseLeftPress.put(ev)
		}
	case termesc.ReleaseButton:
		tpNew := w.textPosFromMouse(ev)
		w.cursorPos = w.textCoordsToWindowCoords(tpNew)
		didSelectWord := false
		// Definition of a double-click: clicking twice on the same character within 0.5 seconds.
		if time.Since(w.lastMouseRelease.when) < doubleClickInterval && !w.lastMouseRelease.isDrag && !w.lastMouseLeftPress.isDrag {
			didSelectWord = w.trySelectWord(w.textPosFromMouse(w.lastMouseRelease.MouseEvent), tpNew)
		}
		if !didSelectWord && w.mouseSelectionAnchor.Set {
			w.selectToCursorPos(&w.mouseSelectionAnchor)
		}
		w.lastMouseRelease.put(ev)
		// If left-press isn't supported, then this always sets isDrag to false, enabling double-clicks
		// to work. If it is, then it should work, unless you drag the mouse into the terminal having
		// clicked outside.
		w.lastMouseRelease.isDrag = w.lastMouseLeftPress.isDrag
		w.wordSelectionAnchor = optionalTextRange{}
	case termesc.ScrollUpButton:
		w.scrollUp()
		w.roundCursorPos()
	case termesc.ScrollDownButton:
		w.scrollDown()
		w.roundCursorPos()
	}
}

func minPoint(p, q buffer.Point) buffer.Point {
	if p.Less(q) {
		return p
	}
	return q
}

func maxPoint(p, q buffer.Point) buffer.Point {
	if p.Less(q) {
		return q
	}
	return p
}

func (w *window) trySelectWord(tpOld, tpNew buffer.Point) bool {
	if tpNew != tpOld {
		return false
	}
	wordBounds := w.buf.WordBoundsAt(tpNew)
	if !wordBounds.Empty() {
		w.selection.Put(wordBounds)
		w.needsRedraw = true
	}
	return !wordBounds.Empty()
}

func (w *window) inMouseSelection() bool {
	return w.mouseSelectionAnchor.Set
}

func (w *window) textPosFromMouse(ev termesc.MouseEvent) point {
	return w.windowCoordsToTextCoords(point{X: ev.X - w.gutterWidth(), Y: ev.Y + w.topLine})
}

func (w *window) cursorPosFromMouse(ev termesc.MouseEvent) point {
	return w.textCoordsToWindowCoords(w.textPosFromMouse(ev))
}

func (w *window) setCursorPosFromMouse(ev termesc.MouseEvent) {
	w.cursorPos.X = ev.X - w.gutterWidth()
	w.cursorPos.Y = ev.Y + w.topLine
	w.roundCursorPos()
}
