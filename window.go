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

	buf *buffer.Buffer // The buffer being edited in the window
}

// Returns the length of line, as visually seen on the console.
func displayLen(line []byte) int {
	n := 0
	for i := 0; i < len(line); {
		p := norm.NFC.NextBoundary(line, true)
		if p == 1 && line[0] == '\t' {
			n += 4
		} else if !(p == 1 && line[0] == '\n') {
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

func (w *window) typeText(text []byte) {
	switch text[0] {
	case '\r':
		w.buf.InsertLineBreak(w.topLine+w.cursorY, w.cursorX)
		w.moveCursorDown()
		w.cursorX = 0
	default:
		w.buf.Insert(text, w.topLine+w.cursorY, w.cursorX)
		w.moveCursorRight()
	}
}

func (w *window) backspace() {
	row := w.topLine + w.cursorY
	newX := 0
	if row > 0 {
		newX = displayLen(w.buf.SliceLines(row-1, row)[0])
	}
	w.buf.DeleteChar(row, w.cursorX)
	if w.cursorX == 0 {
		w.moveCursorUp()
		w.cursorX = newX
	} else {
		w.moveCursorLeft()
	}
}
