// Package buffer implements a text-editing buffer.
package buffer

import (
	"bufio"
	"io"
	"strings"

	"golang.org/x/text/unicode/norm"
)

// Buffer is a text buffer that support efficient access to individual lines of text.
// It implements the io.ReaderFrom and io.WriterTo interfaces.
type Buffer struct {
	lines []string
}

func New() *Buffer { return &Buffer{lines: []string{""}} }

func (b *Buffer) Copy() *Buffer {
	newLines := make([]string, len(b.lines))
	copy(newLines, b.lines)
	return &Buffer{lines: newLines}
}

// ReadFrom reads data from r until EOF, splicing it in at the current insertion point position.
func (b *Buffer) ReadFrom(r io.Reader) (n int64, err error) {
	b.lines = nil
	br := bufio.NewReader(r)
	for {
		var line string
		line, err = br.ReadString('\n')
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
		nw, err := w.Write([]byte(line))
		n += int64(nw)
		if err != nil {
			return n, err
		}
	}
	return n, nil
}

// SliceLines returns the lines of the buffer in the interval [i, j[.
func (b *Buffer) SliceLines(i, j int) []string {
	if j > len(b.lines) {
		j = len(b.lines)
	}
	return b.lines[i:j]
}

// Line returns line i in the buffer.
func (b *Buffer) Line(i int) string {
	if i >= len(b.lines) {
		i = len(b.lines) - 1
	}
	return b.lines[i]
}

// LineCount returns the number of lines in the buffer.
func (b *Buffer) LineCount() int { return len(b.lines) }

func bufIndexForColumn(line string, col int) int {
	i := 0
	p := 0
	for p < len(line) && i < col {
		p += norm.NFC.NextBoundaryInString(line[p:], true)
		i++
	}
	return p
}

var nl = []byte{'\n'}

func (b *Buffer) Insert(text string, row, col int) {
	line := b.lines[row]
	insPoint := bufIndexForColumn(line, col)
	numNewLines := strings.Count(text, "\n")
	if numNewLines > 0 {
		b.lines = append(b.lines, make([]string, numNewLines)...)
		copy(b.lines[row+1+numNewLines:], b.lines[row+1:])
		p := strings.IndexByte(text, '\n')
		carry := line[insPoint:]
		b.lines[row] = line[:insPoint] + text[:p+1]
		text = text[p+1:]
		for i := row + 1; p != -1; i++ {
			newLine := text
			q := strings.IndexByte(text, '\n')
			if q != -1 {
				q = q + 1
				newLine, text = text[:q], text[q:]
			}
			b.lines[i] = newLine
			p = q
		}
		b.lines[row+numNewLines] = b.lines[row+numNewLines] + carry
		return
	}
	b.lines[row] = line[:insPoint] + text + line[insPoint:]
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
	b.lines = append(b.lines, "")
	copy(b.lines[row+1:], b.lines[row:])
	p := bufIndexForColumn(line, col)
	b.lines[row] = line[:p] + "\n"
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
		b.lines[row-1] = prevLine[:len(prevLine)-1] + b.lines[row]
		copy(b.lines[row:], b.lines[row+1:])
		b.lines = b.lines[:len(b.lines)-1]
	} else {
		line := b.lines[row]
		p := bufIndexForColumn(line, col-1)
		n := norm.NFC.NextBoundaryInString(line[p:], true)
		b.lines[row] = line[:p] + line[p+n:]
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
		b.lines[rowStart] = line[:p] + line[q:]
	} else {
		b.lines[rowStart] = b.lines[rowStart][:p] + b.lines[rowEnd][q:]
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
		return []byte(b.lines[rowStart][p:q])
	}
	out := []byte(b.lines[rowStart][p:])
	for i := rowStart + 1; i < rowEnd; i++ {
		out = append(out, b.lines[i]...)
	}
	return append(out, b.lines[rowEnd][:q]...)
}
