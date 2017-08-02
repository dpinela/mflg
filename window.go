package main

import (
	"bytes"
	"fmt"
	"io"

	"github.com/dpinela/mflg/buffer"
)

type window struct {
	w                io.Writer
	width, height    int
	topLine          int //The index of the topmost line being displayed
	cursorX, cursorY int //The cursor position relative to the top left corner of the window

	buf *buffer.Buffer // The buffer being edited in the window
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

// moveCursorDown attempts to move the cursor down by one line and returns a bool indicating
// whether it actually moved.
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

// moveCursorUp attempts to move the cursor up by one line and returns a bool indicating
// whether it actually moved.
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
