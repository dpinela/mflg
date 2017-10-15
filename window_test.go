package main

import (
	"github.com/dpinela/mflg/internal/buffer"

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
		sit[amet] = '🇦🇶'
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
	return newTestWindow(t, 80, 32, testDocument)
}

func checkCursorPos(t *testing.T, stepN int, w *window, p point) {
	if w.cursorPos != p {
		t.Errorf("step %d: cursor at %v, want %v", stepN, w.cursorPos, p)
	}
}

func TestArrowKeyNavigation(t *testing.T) {
	w := newTestWindowA(t)
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
}
