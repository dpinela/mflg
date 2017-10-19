package main

import (
	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/termesc"

	"io/ioutil"
	"strings"
	"testing"
)

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

func newTestWindow(t *testing.T, width, height int, content string) *window {
	buf := buffer.New()
	if _, err := buf.ReadFrom(strings.NewReader(content)); err != nil {
		t.Fatal(err)
	}
	w := newWindow(ioutil.Discard, width, height, buf)
	w.redraw(false)
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
	got := strings.TrimRight(string(w.buf.Line(line)), "\n")
	if got != text {
		t.Errorf("step %d: line %d contains %q, want %q", stepN, line, got, text)
	}
}

// This test is written in such a way that it passes regardless of what
// the tab width is, because calculating the correct results might get
// complicated in some cases.
func TestArrowKeyNavigation(t *testing.T) {
	w := newTestWindowA(t)
	tab := w.tabWidth()
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
		{ev: termesc.MouseEvent{X: 4, Y: 5, Button: termesc.ReleaseButton}, wantPos: point{w.tabWidth(), 5}},
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
	checkCursorPos(t, 1, w, point{1, 9})
	w.moveCursorDown()
	w.moveCursorLeft()
	checkCursorPos(t, 2, w, point{0, 9})
	checkTopLine(t, 2, w, 1)
	for i := 0; i < 4; i++ {
		w.moveCursorDown()
	}
	checkCursorPos(t, 3, w, point{0, 9})
	checkTopLine(t, 3, w, 5)
	w.moveCursorDown()
	checkCursorPos(t, 4, w, point{0, 9})
	checkTopLine(t, 4, w, 5)
	w.moveCursorRight()
	checkCursorPos(t, 5, w, point{1, 9})
	w.moveCursorRight()
	checkCursorPos(t, 5, w, point{1, 9})
	for i := 0; i < 12; i++ {
		w.moveCursorUp()
	}
	checkCursorPos(t, 6, w, point{0, 0})
	checkTopLine(t, 6, w, 2)
}

func TestTextInput(t *testing.T) {
	w := newTestWindowA(t)
	w.cursorPos = point{1, 4}
	w.typeText([]byte("€"))
	checkLineContent(t, 1, w, 4, "á€met consectetur(adìpiscing, elit vestibulum) {")
	checkCursorPos(t, 1, w, point{2, 4})
	w.cursorPos = point{8, 9}
	w.typeText([]byte("$"))
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

func TestLineBreakInput(t *testing.T) {
	w := newTestWindowA(t)
	w.typeText([]byte("\r"))
	checkLineContent(t, 1, w, 0, "")
	checkLineContent(t, 1, w, 1, "#lorem ipsum")
	checkCursorPos(t, 1, w, point{0, 1})
	for i := 0; i < 6; i++ {
		w.moveCursorRight()
	}
	w.typeText([]byte("\r"))
	checkLineContent(t, 2, w, 1, "#lorem")
	checkLineContent(t, 2, w, 2, " ipsum")
	checkCursorPos(t, 2, w, point{0, 2})
	w.moveCursorLeft()
	w.typeText([]byte("\r"))
	checkLineContent(t, 3, w, 1, "#lorem")
	checkLineContent(t, 3, w, 2, "")
	checkLineContent(t, 3, w, 3, " ipsum")
	checkCursorPos(t, 3, w, point{0, 2})
}

func TestBackspace(t *testing.T) {
	const (
		line2      = "dolor sit[10];"
		truncLine4 = "met consectetur(adìpiscing, elit vestibulum) {"
		line5      = "\ttincidunt luctus = sapien + a + porttitor;"
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
	tab := w.tabWidth()
	w.typeText([]byte("\t"))
	checkCursorPos(t, 0, w, point{tab, 0})
	w.typeText([]byte("\r"))
	w.redraw(false)
	//The redraws are needed because the code relies on layout being redone
	// (usually done in the main input loop)
	//after every input; this is not ideal and should change, but we'll
	//leave it this way for now.
	checkLineContent(t, 1, w, 0, "\t")
	checkLineContent(t, 1, w, 1, "\t")
	checkCursorPos(t, 1, w, point{tab, 1})
	for i := 0; i < 3; i++ {
		w.typeText([]byte(" "))
	}
	w.typeText([]byte("\r"))
	w.redraw(false)
	checkLineContent(t, 2, w, 0, "\t")
	checkLineContent(t, 2, w, 1, "\t   ")
	checkLineContent(t, 2, w, 2, "\t   ")
	checkCursorPos(t, 2, w, point{tab + 3, 2})
}
