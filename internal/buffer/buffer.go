// Package buffer implements a text-editing buffer.
package buffer

import (
	"bufio"
	"bytes"
	"io"

	"golang.org/x/text/unicode/norm"
)

// Buffer is a text buffer that support efficient access to individual lines of text.
// It implements the io.ReaderFrom and io.WriterTo interfaces.
type Buffer struct {
	lines [][]byte
}

func New() *Buffer { return &Buffer{lines: [][]byte{nil}} }

// ReadFrom reads data from r until EOF, splicing it in at the current insertion point position.
func (b *Buffer) ReadFrom(r io.Reader) (n int64, err error) {
	b.lines = nil
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

// Line returns line i in the buffer.
func (b *Buffer) Line(i int) []byte {
	if i >= len(b.lines) {
		i = len(b.lines) - 1
	}
	return b.lines[i]
}

// LineCount returns the number of lines in the buffer.
func (b *Buffer) LineCount() int { return len(b.lines) }

func bufIndexForColumn(line []byte, col int) int {
	i := 0
	p := 0
	for p < len(line) && i < col {
		p += norm.NFC.NextBoundary(line[p:], true)
		i++
	}
	return p
}

var nl = []byte{'\n'}

func (b *Buffer) Insert(text []byte, row, col int) {
	line := b.lines[row]
	insPoint := bufIndexForColumn(line, col)
	numNewLines := bytes.Count(text, nl)
	if numNewLines > 0 {
		b.lines = append(b.lines, make([][]byte, numNewLines)...)
		copy(b.lines[row+1+numNewLines:], b.lines[row+1:])
		p := bytes.IndexByte(text, '\n')
		carry := dup(line[insPoint:])
		b.lines[row] = append(line[:insPoint], text[:p+1]...)
		text = text[p+1:]
		for i := row + 1; p != -1; i++ {
			newLine := text
			q := bytes.IndexByte(text, '\n')
			if q != -1 {
				q = q + 1
				newLine, text = text[:q], text[q:]
			}
			b.lines[i] = dup(newLine)
			p = q
		}
		b.lines[row+numNewLines] = append(b.lines[row+numNewLines], carry...)
		return
	}
	line = append(line, make([]byte, len(text))...)
	copy(line[insPoint+len(text):], line[insPoint:])
	copy(line[insPoint:], text)
	b.lines[row] = line
}

func dup(b []byte) []byte {
	c := make([]byte, len(b))
	copy(c, b)
	return c
}

// Returns a copy of b with a newline added at the end.
func dupToLine(b []byte) []byte {
	c := make([]byte, len(b)+1)
	copy(c, b)
	c[len(b)] = '\n'
	return c
}

func (b *Buffer) InsertLineBreak(row, col int) {
	line := b.lines[row]
	b.lines = append(b.lines, nil)
	copy(b.lines[row+1:], b.lines[row:])
	p := bufIndexForColumn(line, col)
	b.lines[row] = dupToLine(line[:p])
	b.lines[row+1] = line[p:]
}

func (b *Buffer) DeleteChar(row, col int) {
	// If we're deleting before the start of a line, concatenate it into the previous one,
	// then remove it.
	if col == 0 {
		if row == 0 {
			return
		}
		prevLine := b.lines[row-1]
		b.lines[row-1] = append(prevLine[:len(prevLine)-1], b.lines[row]...)
		copy(b.lines[row:], b.lines[row+1:])
		b.lines = b.lines[:len(b.lines)-1]
	} else {
		line := b.lines[row]
		p := bufIndexForColumn(line, col-1)
		n := norm.NFC.NextBoundary(line[p:], true)
		copy(line[p:], line[p+n:])
		b.lines[row] = line[:len(line)-n]
	}
}

// DeleteRange deletes all characters in the given range, including line breaks.
// The range is treated as a half-open range.
func (b *Buffer) DeleteRange(rowStart, colStart, rowEnd, colEnd int) {
	if rowEnd < rowStart || (rowStart == rowEnd && colEnd < colStart) {
		rowStart, rowEnd = rowEnd, rowStart
		colStart, colEnd = colEnd, colStart
	}
	p := bufIndexForColumn(b.lines[rowStart], colStart)
	q := bufIndexForColumn(b.lines[rowEnd], colEnd)
	if rowStart == rowEnd {
		line := b.lines[rowStart]
		copy(line[p:], line[q:])
		b.lines[rowStart] = line[:len(line)-(q-p)]
	} else {
		lineStart := b.lines[rowStart]
		lineEnd := b.lines[rowEnd]
		b.lines[rowStart] = append(lineStart[:p], lineEnd[q:]...)
		// Delete all the lines entirely between the start and the end point;
		// the line where the end point lies is deleted too, since it was
		// merged into the start line.
		copy(b.lines[rowStart+1:], b.lines[rowEnd+1:])
		b.lines = b.lines[:len(b.lines)-(rowEnd-rowStart)]
	}
}

// CopyRange returns a copy of the characters in the given range, as a
// contiguous slice.
func (b *Buffer) CopyRange(rowStart, colStart, rowEnd, colEnd int) []byte {
	p := bufIndexForColumn(b.lines[rowStart], colStart)
	q := bufIndexForColumn(b.lines[rowEnd], colEnd)
	if rowStart == rowEnd {
		return dup(b.lines[rowStart][p:q])
	}
	out := dup(b.lines[rowStart][p:])
	for i := rowStart + 1; i < rowEnd; i++ {
		out = append(out, b.lines[i]...)
	}
	return append(out, b.lines[rowEnd][:q]...)
}
