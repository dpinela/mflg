package main

import (
	"bytes"
	"fmt"
	"io"

	"github.com/dpinela/mflg/buffer"
	"golang.org/x/text/unicode/norm"
)

type window struct {
	w                io.Writer
	width, height    int
	topLine          int //The index of the topmost line being displayed
	cursorX, cursorY int //The cursor position relative to the top left corner of the window

	dirty bool //Indicates whether the contents of the window's buffer have been modified

	buf *buffer.Buffer // The buffer being edited in the window
}

// Returns the length of line, as visually seen on the console.
func displayLen(line []byte) int {
	n := 0
	for i := 0; i < len(line); {
		p := norm.NFC.NextBoundary(line, true)
		if p == 1 && line[0] == '\n' {
			break
		} else if p == 1 && line[0] == '\t' {
			n += 4
		} else {
			n++
		}
		line = line[p:]
	}
	return n
}

func (w *window) renderBuffer() error {
	lines := w.buf.SliceLines(w.topLine, w.topLine+w.height)
	const gutterSize = 4
	for i, line := range lines {
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		line = truncateToWidth(bytes.Replace(line, tab, fourSpaces, -1), w.width-gutterSize)
		if _, err := fmt.Fprintf(w.w, "% 3d ", w.topLine+i+1); err != nil {
			return err
		}
		if _, err := w.w.Write(line); err != nil {
			return err
		}
		if i+1 < w.height {
			if _, err := w.w.Write(crlf); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *window) moveCursorDown() {
	switch {
	case w.cursorY < w.height-1:
		w.cursorY++
	case w.topLine < w.buf.LineCount():
		w.topLine++
	default:
		mustWrite(w.w, []byte("\a"))
	}
}

func (w *window) moveCursorUp() {
	switch {
	case w.cursorY > 0:
		w.cursorY--
	case w.topLine > 0:
		w.topLine--
	default:
		mustWrite(w.w, []byte("\a"))
	}
}

func (w *window) moveCursorLeft() {
	if w.cursorX > 0 {
		w.cursorX--
	}
}

func (w *window) moveCursorRight() {
	if w.cursorX < w.width-1 {
		w.cursorX++
	}
}

// Window coordinates: a (y, x) position within the window.
// Text coordinates: a (line, column) position within the text.

func (w *window) windowCoordsToTextCoords(wy, wx int) (ty int, tx int) {
	ty = w.topLine + wy
	if ty >= w.buf.LineCount() {
		ty = w.buf.LineCount() - 1
	}
	line := w.buf.SliceLines(ty, ty+1)[0]
	for n := 0; len(line) != 0 && n < wx; {
		p := norm.NFC.NextBoundary(line, true)
		if p == 1 && line[0] == '\t' {
			n += 4
		} else if p == 1 && line[0] == '\n' {
			break
		} else {
			n++
		}
		tx++
		line = line[p:]
	}
	return ty, tx
}

func (w *window) typeText(text []byte) {
	w.dirty = true
	y, x := w.windowCoordsToTextCoords(w.cursorY, w.cursorX)
	switch text[0] {
	case '\r':
		w.buf.InsertLineBreak(y, x)
		w.moveCursorDown()
		w.cursorX = 0
	default:
		w.buf.Insert(text, y, x)
		w.moveCursorRight()
	}
}

func (w *window) backspace() {
	w.dirty = true
	y, x := w.windowCoordsToTextCoords(w.cursorY, w.cursorX)
	newX := 0
	if y > 0 {
		newX = displayLen(w.buf.SliceLines(y-1, y)[0])
	}
	w.buf.DeleteChar(y, x)
	if w.cursorX == 0 {
		w.moveCursorUp()
		w.cursorX = newX
	} else {
		w.moveCursorLeft()
	}
}

var gotoBottomAndClear = []byte("\033[1;2000B\033[K")

func (w *window) printAtBottom(text string) error {
	gotoPos(w.w, 2000, 0)
	_, err := w.w.Write([]byte(text))
	return err
}
