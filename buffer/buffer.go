// Package buffer implements a text-editing buffer.
package buffer

import (
	"bufio"
	"io"
)

// Buffer is a text buffer that support efficient access to individual lines of text.
// It implements the io.ReaderFrom and io.WriterTo interfaces.
type Buffer struct {
	lines [][]byte
}

func New() *Buffer { return &Buffer{lines: nil} }

// ReadFrom reads data from r until EOF, splicing it in at the current insertion point position.
func (b *Buffer) ReadFrom(r io.Reader) (n int64, err error) {
	br := bufio.NewReader(r)
	for {
		var line []byte
		line, err = br.ReadBytes('\n')
		b.lines = append(b.lines, line)
		if err != nil {
			if err == io.EOF {
				err = nil
			}
			return
		}
		n += int64(len(line))
	}
}

// WriteTo writes the full content of the buffer to w.
func (b *Buffer) WriteTo(w io.Writer) (int64, error) {
	var n int64
	for _, line := range b.lines {
		nw, err := w.Write(line)
		n += int64(nw)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// SliceLines returns the lines of the buffer in the interval [i, j[.
func (b *Buffer) SliceLines(i, j int) [][]byte {
	if j > len(b.lines) {
		j = len(b.lines)
	}
	return b.lines[i:j]
}
