package main

import (
	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/clipboard"
	"github.com/dpinela/mflg/internal/termesc"

	"bytes"
	"regexp"
	"strings"
	"testing"
	"time"
)

func init() {
	// Important for undo tests.
	changeCoalescingInterval = time.Millisecond
}

const testDocument = `#lorem ipsum

dolor sit[10];

ámet consectetur(adìpiscing, elit vestibulum) {
	tincidunt luctus = sapien + a + porttitor;
	massa dapibus > sit[amet] {
		donec("venenatis %d:%d\n", sit.amet, eros.vitae);
		ullamcorper nunc a("henderit magna: donec est mi, viverra in aliquet quis");
	}
	eleifend {
		sit[amet] = 'q';
	}
}`

func newTestWindow(t testing.TB, width, height int, content string) *window {
	buf := buffer.New()
	if _, err := buf.ReadFrom(strings.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	w := newWindow(width, height, buf, 4)
	return w
}

func newTestWindowA(t *testing.T) *window {
	return newTestWindow(t, 80, 10, testDocument)
}

func newTestWindowEmpty(t *testing.T) *window {
	return newTestWindow(t, 80, 10, "")
}

func checkCursorPos(t *testing.T, stepN int, w *window, p point) {
	t.Helper()
	if w.cursorPos != p {
		t.Errorf("step %d: cursor at %v, want %v", stepN, w.cursorPos, p)
	}
}

func checkTopLine(t *testing.T, stepN int, w *window, n int) {
	t.Helper()
	if w.topLine != n {
		t.Errorf("step %d: topLine = %v, want %v", stepN, w.topLine, n)
	}
}

func checkLineContent(t *testing.T, stepN int, w *window, line int, text string) {
	t.Helper()
	if got := strings.TrimRight(w.buf.Line(line), "\n"); got != text {
		t.Errorf("step %d: line %d contains %q, want %q", stepN, line, got, text)
	}
}

func checkBufContent(t *testing.T, buf *buffer.Buffer, text string) {
	t.Helper()
	var b strings.Builder
	if _, err := buf.WriteTo(&b); err != nil {
		t.Error(err)
	}
	if got := b.String(); got != text {
		t.Errorf("buffer contains %q, want %q", got, text)
	}
}

func checkWrappedLine(t *testing.T, w *window, y int, want buffer.WrappedLine) {
	t.Helper()
	if got := w.wrappedBuf.Line(y); got != want {
		t.Errorf("line %d contains %q at (%d, %d), want %q at (%d, %d)", y, got.Text, got.Start.X, got.Start.Y, want.Text, want.Start.X, want.Start.Y)
	}
}

func checkNeedsRedraw(t *testing.T, w *window) {
	t.Helper()
	if !w.needsRedraw {
		t.Error("no redraw requested")
	}
}

// This test is written in such a way that it passes regardless of what
// the tab width is, because calculating the correct results might get
// complicated in some cases.
func TestArrowKeyNavigation(t *testing.T) {
	w := newTestWindowA(t)
	tab := w.getTabWidth()
	checkCursorPos(t, 0, w, point{0, 0})
	w.moveCursorLeft()
	checkCursorPos(t, 1, w, point{0, 0})
	w.moveCursorRight()
	w.moveCursorRight()
	checkCursorPos(t, 2, w, point{2, 0})
	w.moveCursorDown()
	checkCursorPos(t, 3, w, point{0, 1})
	w.moveCursorUp()
	checkCursorPos(t, 4, w, point{0, 0})
	for i := 0; i < 4; i++ {
		w.moveCursorDown()
	}
	w.moveCursorRight()
	checkCursorPos(t, 5, w, point{1, 4})
	w.moveCursorDown()
	checkCursorPos(t, 6, w, point{tab, 5})
	w.moveCursorLeft()
	w.moveCursorLeft()
	checkCursorPos(t, 7, w, point{47, 4})
	w.moveCursorRight()
	checkCursorPos(t, 8, w, point{0, 5})
	for i := 0; i < 3; i++ {
		w.moveCursorDown()
	}
	w.moveCursorRight()
	w.moveCursorRight()
	w.moveCursorDown()
	checkCursorPos(t, 9, w, point{2 * tab, 9})
}

func TestRightArrowOffscreen(t *testing.T) {
	w := newTestWindow(t, 80, 25, strings.Repeat(testDocument, 300))
	w.topLine = 100
	w.moveCursorRight()
	checkTopLine(t, 1, w, 0)
	checkCursorPos(t, 1, w, point{1, 0})
}

func TestArrowKeyWordNavigation(t *testing.T) {
	w := newTestWindowA(t)
	w.moveCursorRightWord()
	checkCursorPos(t, 0, w, point{1, 0})
	w.moveCursorRightWord()
	checkCursorPos(t, 1, w, point{6, 0})
	w.moveCursorRightWord()
	checkCursorPos(t, 2, w, point{7, 0})
	w.moveCursorRightWord()
	checkCursorPos(t, 3, w, point{12, 0})
	w.moveCursorRightWord()
	checkCursorPos(t, 4, w, point{0, 1})
	w.moveCursorLeftWord()
	checkCursorPos(t, 5, w, point{12, 0})
	w.moveCursorLeftWord()
	checkCursorPos(t, 6, w, point{7, 0})
	w.moveCursorLeftWord()
	checkCursorPos(t, 7, w, point{6, 0})
	w.moveCursorLeftWord()
	checkCursorPos(t, 8, w, point{1, 0})
	// This seems a bit uninuitive, but is a logical result of the definition of word boundaries.
	w.moveCursorLeftWord()
	checkCursorPos(t, 9, w, point{1, 0})
}

func TestMouseNavigation(t *testing.T) {
	w := newTestWindow(t, 80, 50, testDocument)
	var mouseNavTests = []struct {
		ev      termesc.MouseEvent
		wantPos point
	}{
		{ev: termesc.MouseEvent{X: 16, Y: 2, Button: termesc.ReleaseButton}, wantPos: point{13, 2}},
		// Tests for out of bounds clicks
		{ev: termesc.MouseEvent{X: 50, Y: 0, Button: termesc.ReleaseButton}, wantPos: point{12, 0}},
		{ev: termesc.MouseEvent{X: 45, Y: 22, Button: termesc.ReleaseButton}, wantPos: point{1, 14}},
		// Click inside a tab
		{ev: termesc.MouseEvent{X: 4, Y: 5, Button: termesc.ReleaseButton}, wantPos: point{w.getTabWidth(), 5}},
	}
	for _, tt := range mouseNavTests {
		w.handleMouseEvent(tt.ev)
		if w.cursorPos != tt.wantPos {
			t.Errorf("click at (%d, %d): got cursor at %v, want %v", tt.ev.X, tt.ev.Y, w.cursorPos, tt.wantPos)
		}
	}
}

func TestScrolling(t *testing.T) {
	w := newTestWindowA(t)
	for i := 0; i < 9; i++ {
		w.moveCursorDown()
	}
	checkCursorPos(t, 1, w, point{0, 9})
	w.moveCursorDown()
	w.moveCursorLeft()
	checkCursorPos(t, 2, w, point{8, 9})
	checkTopLine(t, 2, w, 1)
	for i := 0; i < 4; i++ {
		w.moveCursorDown()
	}
	checkCursorPos(t, 3, w, point{w.getTabWidth() + 1, 13})
	checkTopLine(t, 3, w, 4)
	w.moveCursorDown()
	checkCursorPos(t, 4, w, point{1, 14})
	checkTopLine(t, 4, w, 5)
	w.moveCursorRight()
	checkCursorPos(t, 5, w, point{1, 14})
	for i := 0; i < 12; i++ {
		w.moveCursorUp()
	}
	checkCursorPos(t, 6, w, point{0, 2})
	checkTopLine(t, 6, w, 2)
}

func TestResize(t *testing.T) {
	w := newTestWindowA(t)
	w.moveCursorDown()
	w.moveCursorDown()
	w.resize(10, 6)
	checkCursorPos(t, 1, w, point{0, 2})
	w.resize(1, 6)
	checkCursorPos(t, 2, w, point{0, 2})
}

func TestScrollingOneLine(t *testing.T) {
	const (
		gutterText  = "X"
		windowWidth = 20
	)
	w := newTestWindow(t, windowWidth, 1, strings.Repeat("A", 2000))
	// Ensure the gutter is 2 units wide
	w.setGutterText(gutterText)
	w.moveCursorDown()
	checkCursorPos(t, 1, w, point{0, 1})
	checkTopLine(t, 1, w, 1)
	w.topLine = 0
	w.cursorPos = point{w.textAreaWidth() - 1, 0}
	w.needsRedraw = false
	w.moveCursorRight()
	checkCursorPos(t, 2, w, point{0, 1})
	checkTopLine(t, 2, w, 1)
	checkNeedsRedraw(t, w)
	w.needsRedraw = false
	w.moveCursorLeft()
	checkCursorPos(t, 3, w, point{w.textAreaWidth() - 1, 0})
	checkTopLine(t, 3, w, 0)
	checkNeedsRedraw(t, w)
}

func TestTextInput(t *testing.T) {
	w := newTestWindowA(t)
	w.cursorPos = point{1, 4}
	w.typeText("€")
	checkLineContent(t, 1, w, 4, "á€met consectetur(adìpiscing, elit vestibulum) {")
	checkCursorPos(t, 1, w, point{2, 4})
	w.cursorPos = point{8, 9}
	w.typeText("$")
	checkLineContent(t, 2, w, 8, "\t\tullamcorper nunc a(\"henderit magna: donec est mi, viverra in aliquet quis\");$")
	checkCursorPos(t, 2, w, point{9, 9})
	checkTopLine(t, 2, w, 0)
	/*
		checkLineContent(t, 0, w, 0, "#lorem ipsum")
		w.typeText([]byte("#"))
		checkLineContent(t, 1, w, 0, "##lorem ipsum")
		checkCursorPos(t, 1, w, point{1, 0})
		w.typeText([]byte("€"))
		checkLineContent(t, 2, w, 0, "#€#lorem ipsum")
		checkCursorPos(t, 2, w, point{2, 0})
		w.typeText([]byte("🇦🇶"))
		checkLineContent(t, 3, w, 0, "#€🇦🇶#lorem ipsum")
		checkCursorPos(t, 3, w, point{3, 0})
		w.typeText([]byte("a"))
		checkLineContent(t, 3, w, 0, "#€🇦🇶a#lorem ipsum")*/
}

func TestWideCharNavigation(t *testing.T) {
	w := newTestWindowEmpty(t)
	w.typeText("ヲ")
	checkCursorPos(t, 1, w, point{2, 0})
	w.moveCursorLeft()
	checkCursorPos(t, 2, w, point{0, 0})
	w.moveCursorRight()
}

func TestLineBreakInput(t *testing.T) {
	w := newTestWindowA(t)
	w.typeText("\r")
	checkLineContent(t, 1, w, 0, "")
	checkLineContent(t, 1, w, 1, "#lorem ipsum")
	checkCursorPos(t, 1, w, point{0, 1})
	for i := 0; i < 6; i++ {
		w.moveCursorRight()
	}
	w.typeText("\r")
	checkLineContent(t, 2, w, 1, "#lorem")
	checkLineContent(t, 2, w, 2, " ipsum")
	checkCursorPos(t, 2, w, point{0, 2})
	w.moveCursorLeft()
	w.typeText("\r")
	checkLineContent(t, 3, w, 1, "#lorem")
	checkLineContent(t, 3, w, 2, "")
	checkLineContent(t, 3, w, 3, " ipsum")
	checkCursorPos(t, 3, w, point{0, 2})
}

const line5 = "\ttincidunt luctus = sapien + a + porttitor;"

func TestBackspace(t *testing.T) {
	const (
		line2      = "dolor sit[10];"
		truncLine4 = "met consectetur(adìpiscing, elit vestibulum) {"
	)

	w := newTestWindowA(t)
	w.cursorPos = point{1, 4}
	w.backspace()
	checkLineContent(t, 1, w, 4, truncLine4)
	checkLineContent(t, 1, w, 3, "")
	checkCursorPos(t, 1, w, point{0, 4})
	w.backspace()
	checkCursorPos(t, 2, w, point{0, 3})
	checkLineContent(t, 2, w, 2, line2)
	checkLineContent(t, 2, w, 3, truncLine4)
	checkLineContent(t, 2, w, 4, line5)
	w.backspace()
	checkCursorPos(t, 3, w, point{len(line2), 2})
	checkLineContent(t, 3, w, 2, line2+truncLine4)
	checkLineContent(t, 3, w, 3, line5)
}

func TestAutoIndent(t *testing.T) {
	w := newTestWindowEmpty(t)
	tab := w.getTabWidth()
	w.typeText("\t")
	checkCursorPos(t, 0, w, point{tab, 0})
	w.typeText("\r")
	checkLineContent(t, 1, w, 0, "\t")
	checkLineContent(t, 1, w, 1, "\t")
	checkCursorPos(t, 1, w, point{tab, 1})
	for i := 0; i < 3; i++ {
		w.typeText(" ")
	}
	w.typeText("\r")
	checkLineContent(t, 2, w, 0, "\t")
	checkLineContent(t, 2, w, 1, "\t   ")
	checkLineContent(t, 2, w, 2, "\t   ")
	checkCursorPos(t, 2, w, point{tab + 3, 2})
}

func TestTabWidthDetection(t *testing.T) {
	w := newTestWindow(t, 100, 100, `
func main() {
  if u := os.Getenv("USER"); u == "root" {
    fmt.Println("Careful!")
  } else {
    fmt.Println("Hello," u)
  }
}`)
	const (
		M     = 2
		chunk = "func main() {"
	)
	if n := w.getTabWidth(); n != M {
		t.Errorf("got tabWidth() = %d, want %d", n, M)
	}
	w.cursorPos = point{0, 1}
	w.typeText("\t")
	checkCursorPos(t, 1, w, point{M, 1})
	checkLineContent(t, 1, w, 1, strings.Repeat(" ", M)+chunk)
	w.backspace()
	checkCursorPos(t, 2, w, point{1, 1})
	checkLineContent(t, 2, w, 1, strings.Repeat(" ", M-1)+chunk)
}

func TestCutNothing(t *testing.T) {
	w := newTestWindowA(t)
	p := point{1, 0}
	w.cursorPos = p
	w.cutSelection()
	checkCursorPos(t, 1, w, p)
	checkLineContent(t, 1, w, 0, "#lorem ipsum")
}

func TestCopy(t *testing.T) {
	const timeout = 100 * time.Millisecond
	w := newTestWindowA(t)
	w.selection.Put(textRange{point{0, 0}, point{5, 2}})
	w.copySelection()
	time.Sleep(timeout)
	const wantData = "#lorem ipsum\n\ndolor"
	data, err := clipboard.Paste()
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != wantData {
		t.Errorf("copy then paste after %v: got %q, want %q", timeout, data, wantData)
	}
}

func TestPaste(t *testing.T) {
	const (
		chunk = "blub"
		x     = 6
	)
	w := newTestWindowA(t)
	w.cursorPos = point{x, 0}
	if err := clipboard.Copy([]byte(chunk)); err != nil {
		t.Fatal(err)
	}
	w.paste()
	checkCursorPos(t, 1, w, point{x + len(chunk), 0})
	checkLineContent(t, 1, w, 0, "#lorem"+chunk+" ipsum")
}

func TestPasteWideChar(t *testing.T) {
	const chunk = "漢字"
	w := newTestWindowEmpty(t)
	if err := clipboard.Copy([]byte(chunk)); err != nil {
		t.Fatal(err)
	}
	w.paste()
	checkCursorPos(t, 1, w, point{4, 0})
	checkLineContent(t, 1, w, 0, chunk)
}

/*func TestPasteMultiline(t *testing.T) {
	const chunk = "blub\nblub\nblub"
	w := newTestWindow(t, 160, 20, testDocument)
	w.cursorPos = point{w.getTabWidth() + 2, 10}
	if err := clipboard.Copy([]byte(chunk)); err != nil {
		t.Fatal(err)
	}
	w.paste()
	checkCursorPos(t, 1, w, point{w.getTabWidth() + 4, 12})
	checkLineContent(t, 1, w, 10, "\telblub")
	checkLineContent(t, 1, w, 11, "\tblub")
	checkLineContent(t, 1, w, 12, "\tblubeifend {")
}*/

func TestKeyboardSelection(t *testing.T) {
	wantSelection := optionalTextRange{textRange{point{0, 2}, point{5, 2}}, true}

	w := newTestWindowA(t)
	w.cursorPos = point{0, 2}
	w.markSelectionBound()
	w.cursorPos = point{5, 2}
	w.markSelectionBound()
	if w.selection != wantSelection {
		t.Errorf("got selection %+v, want %+v", w.selection, wantSelection)
	}
}

func TestTwoKeyboardSelections(t *testing.T) {
	w := newTestWindowA(t)
	w.cursorPos = point{0, 2}
	w.markSelectionBound()
	w.cursorPos = point{5, 2}
	w.markSelectionBound()
	checkSelection(t, 1, w, testSelection)
	w.cursorPos = point{6, 2}
	w.markSelectionBound()
	checkSelection(t, 2, w, optionalTextRange{})
	w.cursorPos = point{0, 2}
	w.markSelectionBound()
	checkSelection(t, 3, w, optionalTextRange{textRange{point{0, 2}, point{6, 2}}, true})
}

func TestMouseSelection(t *testing.T) {
	w := newTestWindowA(t)
	testMouseSelection(t, w)
}

func TestMouseSelectionOverride(t *testing.T) {
	w := newTestWindowA(t)
	w.cursorPos = point{8, 6}
	w.markSelectionBound()
	testMouseSelection(t, w)
}

func TestCancelMouseSelection(t *testing.T) {
	w := newTestWindowA(t)
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.LeftButton, X: 3, Y: 2})
	w.resetSelectionState()
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.ReleaseButton, X: 8, Y: 2})
	if w.selection.Set {
		t.Errorf("got selection %+v, want none", w.selection)
	}
}

func TestClearMouseSelection(t *testing.T) {
	w := newTestWindowA(t)
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.LeftButton, X: 3, Y: 2})
	// Clearing a mouse selection only works if we can distinguish a drag from a non-drag. On other
	// tests it doesn't matter, but here we have to simulate a drag more accurately to properly test
	// the clearing code.
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.LeftButton, X: 5, Y: 2, Move: true})
	c2 := termesc.MouseEvent{Button: termesc.ReleaseButton, X: 8, Y: 2}
	c3 := c2
	c3.Button = termesc.LeftButton
	w.handleMouseEvent(c2)
	w.handleMouseEvent(c3)
	w.handleMouseEvent(c2)
	if w.selection.Set {
		t.Errorf("got selection %+v, want none", w.selection)
	}
}

func TestWordSelection(t *testing.T) {
	w := newTestWindowA(t)
	w.needsRedraw = false
	doubleClick(w, 4, 2)
	checkSelection(t, 1, w, optionalTextRange{textRange{point{0, 2}, point{5, 2}}, true})
	checkNeedsRedraw(t, w)
}

// Test for selection of a word in a buffer containing only that word and nothing else.
func TestLoneWordSelection(t *testing.T) {
	w := newTestWindow(t, 80, 3, "dégradé")
	w.needsRedraw = false
	doubleClick(w, 4, 0)
	checkSelection(t, 1, w, optionalTextRange{textRange{point{0, 0}, point{7, 0}}, true})
	checkNeedsRedraw(t, w)
}

func TestFailedWordSelection(t *testing.T) {
	w := newTestWindowA(t)
	w.needsRedraw = false
	doubleClick(w, 9, 0)
	checkSelection(t, 1, w, optionalTextRange{})
	// FIXME: This is consistent with the single-click behaviour (clicking inside a tab goes to the next character),
	// but maybe that should change (so that clicking on a character always puts the cursor before that
	// character).
	//doubleClick(w, 4, 5)
	//checkSelection(t, 2, w, optionalTextRange{})
}

func doubleClick(w *window, x, y int) {
	for i := 0; i < 2; i++ {
		w.handleMouseEvent(termesc.MouseEvent{Button: termesc.ReleaseButton, X: x, Y: y})
	}
}

func TestWordSelectionDrag(t *testing.T) {
	w := newTestWindowA(t)
	w.needsRedraw = false
	click := termesc.MouseEvent{Button: termesc.LeftButton, X: 4, Y: 2}
	release := click
	release.Button = termesc.ReleaseButton
	w.handleMouseEvent(click)
	w.handleMouseEvent(release)
	w.handleMouseEvent(click)
	checkSelection(t, 1, w, optionalTextRange{textRange{point{0, 2}, point{5, 2}}, true})
	checkNeedsRedraw(t, w)
	w.needsRedraw = false
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.LeftButton, X: 9, Y: 2, Move: true})
	checkSelection(t, 2, w, optionalTextRange{textRange{point{0, 2}, point{9, 2}}, true})
	checkNeedsRedraw(t, w)
	w.needsRedraw = false
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.LeftButton, X: 10, Y: 4, Move: true})
	checkSelection(t, 3, w, optionalTextRange{textRange{point{0, 2}, point{16, 4}}, true})
	checkNeedsRedraw(t, w)
	w.needsRedraw = false
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.LeftButton, X: 56, Y: 4, Move: true})
	checkSelection(t, 4, w, optionalTextRange{textRange{point{0, 2}, point{47, 4}}, true})
	checkNeedsRedraw(t, w)
}

var testSelection = optionalTextRange{textRange{point{0, 2}, point{5, 2}}, true}

func checkSelection(t *testing.T, step int, w *window, want optionalTextRange) {
	t.Helper()
	if w.selection != want {
		t.Errorf("step %d: got selection %v, want %v", step, w.selection, want)
	}
}

func testMouseSelection(t *testing.T, w *window) {
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.LeftButton, X: 3, Y: 2})
	checkSelection(t, 1, w, optionalTextRange{})
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.LeftButton, Move: true, X: 4, Y: 2})
	checkSelection(t, 2, w, optionalTextRange{textRange{point{0, 2}, point{1, 2}}, true})
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.LeftButton, Move: true, X: 8, Y: 2})
	checkSelection(t, 3, w, testSelection)
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.ReleaseButton, X: 8, Y: 2})
	checkSelection(t, 4, w, testSelection)
}

func TestHybridSelection(t *testing.T) {
	w := newTestWindowA(t)
	w.cursorPos = point{0, 2}
	w.markSelectionBound()
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.LeftButton, X: 8, Y: 2})
	w.handleMouseEvent(termesc.MouseEvent{Button: termesc.ReleaseButton, X: 8, Y: 2})
	w.markSelectionBound()
	if w.selection != testSelection {
		t.Errorf("got selection %+v, want %+v", w.selection, testSelection)
	}
}

func TestBackspaceSelection(t *testing.T) {
	w := newTestWindowA(t)
	w.selection.Put(textRange{point{0, 2}, point{5, 2}})
	w.backspace()
	checkLineContent(t, 1, w, 1, "")
	checkLineContent(t, 1, w, 2, " sit[10];")
	w.selection.Put(textRange{point{0, 0}, point{5, 4}})
	w.backspace()
	checkLineContent(t, 2, w, 0, "consectetur(adìpiscing, elit vestibulum) {")
	checkLineContent(t, 2, w, 1, line5)
}

func TestOverwriteSelection(t *testing.T) {
	w := newTestWindowA(t)
	w.selection.Put(textRange{point{1, 0}, point{7, 0}})
	w.typeText("#")
	checkLineContent(t, 1, w, 0, "##ipsum")
}

func TestDownFromFullLine(t *testing.T) {
	w := newTestWindow(t, 16, 5, testDocument)
	w.moveCursorDown()
	checkCursorPos(t, 1, w, point{0, 1})
}

var controlSeqRE = regexp.MustCompile(`\x1b\[[^a-zA-Z]*[a-zA-Z]`)

func TestRenderOneLine(t *testing.T) {
	const testOutLine = "OL: ABCD"
	w := newTestWindow(t, 9, 1, "ABCDEFGH")
	w.setGutterText("OL:")
	var fakeConsole bytes.Buffer
	if err := w.redraw(&fakeConsole); err != nil {
		t.Error(err)
	}
	if out := controlSeqRE.ReplaceAllString(fakeConsole.String(), ""); out != testOutLine {
		t.Errorf("got %q, want %q", out, testOutLine)
	}
}

func TestGutterResize(t *testing.T) {
	w := newTestWindow(t, 9, 15, "a\nb\nc\nd\nefghij\nl\nm\nn\no")
	w.typeText("\r")
	checkWrappedLine(t, w, 5, buffer.WrappedLine{Start: buffer.Point{X: 0, Y: 5}, Text: "efghi"})
	checkWrappedLine(t, w, 6, buffer.WrappedLine{Start: buffer.Point{X: 5, Y: 5}, Text: "j\n"})
}

const shortTestDocument = `func A() int { return 4 }
func Go() int { return 5 }`

var replaceTestRegexp = regexp.MustCompile(`func (\w+)`)

func TestReplace(t *testing.T) {
	w := newTestWindow(t, 10, 10, shortTestDocument)
	w.replaceRegexp(replaceTestRegexp, "function $1$1")
	checkLineContent(t, 1, w, 0, "function AA() int { return 4 }")
	checkLineContent(t, 1, w, 1, "function GoGo() int { return 5 }")
}

func TestReplaceInSelection(t *testing.T) {
	w := newTestWindow(t, 10, 10, shortTestDocument+"\n"+shortTestDocument)

	w.selection.Put(buffer.Range{point{7, 1}, point{11, 2}})
	w.replaceRegexp(replaceTestRegexp, "function $1$1")
	checkLineContent(t, 1, w, 0, "func A() int { return 4 }")
	checkLineContent(t, 1, w, 1, "func Go() int { return 5 }")
	checkLineContent(t, 1, w, 2, "function AA() int { return 4 }")
	checkLineContent(t, 1, w, 3, "func Go() int { return 5 }")
	checkSelection(t, 1, w, optionalTextRange{buffer.Range{point{7, 1}, point{16, 2}}, true})
	/*
		newSelection := buffer.Range{point{0, 0}, point{11, 0}}
		w.selection.Put(newSelection)
		w.replaceRegexp(regexp.MustCompile("int"), "float32")
		checkLineContent(t, 2, w, 0, "func A() int { return 4 }")
		checkSelection(t, 2, w, optionalTextRange{newSelection, true})*/
}

const (
	undoneText1 = "boom!"
	undoneText2 = " HO! HO! HO!"
)

func TestUndo(t *testing.T) {
	w := newTestWindowEmpty(t)
	typeStringsWithPause(w, undoneText1, undoneText2, " Merry undoing!")
	w.undo()
	checkLineContent(t, 1, w, 0, undoneText1+undoneText2)
	checkCursorPos(t, 1, w, point{X: len(undoneText1) + len(undoneText2), Y: 0})
	w.undo()
	checkLineContent(t, 2, w, 0, undoneText1)
	checkCursorPos(t, 2, w, point{X: len(undoneText1), Y: 0})
	w.undo()
	checkLineContent(t, 3, w, 0, "")
	checkCursorPos(t, 3, w, point{X: 0, Y: 0})
}

func TestUndoAll(t *testing.T) {
	w := newTestWindow(t, 20, 10, shortTestDocument)
	typeStringsWithPause(w, undoneText1, undoneText2)
	w.undoAll()
	checkBufContent(t, w.buf, shortTestDocument)
	checkCursorPos(t, 1, w, point{X: 0, Y: 0})
}

func typeString(w *window, s string) {
	for _, c := range s {
		w.typeText(string(c))
	}
}

func typeStringsWithPause(w *window, strings ...string) {
	for _, s := range strings {
		time.Sleep(2 * time.Millisecond)
		typeString(w, s)
	}
}

func TestUndoWithSelection(t *testing.T) {
	w := newTestWindow(t, 20, 10, shortTestDocument)
	selectionEnd := point{9, 1}
	selection := buffer.Range{point{4, 0}, selectionEnd}
	w.cursorPos = selectionEnd
	w.selection.Put(selection)
	w.typeText("B")
	w.undo()
	checkLineContent(t, 1, w, 0, "func A() int { return 4 }")
	checkLineContent(t, 1, w, 1, "func Go() int { return 5 }")
	checkCursorPos(t, 1, w, selectionEnd)
	checkSelection(t, 1, w, optionalTextRange{Set: true, textRange: selection})
}

func TestUndoNothing(t *testing.T) {
	w := newTestWindow(t, 20, 10, shortTestDocument)
	p := point{10, 1}
	w.cursorPos = p
	w.undo()
	checkLineContent(t, 1, w, 0, "func A() int { return 4 }")
	checkLineContent(t, 1, w, 1, "func Go() int { return 5 }")
	checkCursorPos(t, 1, w, p)
}
