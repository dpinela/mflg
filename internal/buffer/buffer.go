// Package buffer implements a text-editing buffer.
package buffer

import (
	"bufio"
	"io"
	"strings"

	"golang.org/x/text/unicode/norm"
	"unicode"
	"unicode/utf8"
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

// Indicates a buffer indented with tabs.
const IndentTabs = 0

// IndentType returns the number of spaces used for each leading indentation level in the
// text, or IndentTabs if the text is indented using tabs.
// If it cannot determine the indentation type, returns IndentTabs.
func (b *Buffer) IndentType() int {
	multiplesSeen := make([]int, 32)
lineScan:
	for _, line := range b.lines {
		numSpaces := 0
		hasTabs := false
	prefixScan:
		for i := range line {
			switch line[i] {
			case '\t':
				if numSpaces > 0 {
					// If we run into a line that mixes tabs and spaces, just ignore that line
					// and hope to use the rest to find out what we need.
					continue lineScan
				}
				hasTabs = true
			case ' ':
				if hasTabs {
					continue lineScan
				}
				numSpaces++
			default:
				break prefixScan
			}
		}
		switch {
		case hasTabs:
			multiplesSeen[0]++
		case numSpaces > 0:
			for i := 1; i < len(multiplesSeen); i++ {
				if numSpaces%i == 0 {
					multiplesSeen[i]++
				}
			}
		}
	}
	best := IndentTabs
	bestCount := 0
	for i, n := range multiplesSeen {
		if n >= bestCount {
			best = i
			bestCount = n
		}
	}
	if bestCount > 0 {
		return best
	}
	return IndentTabs
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

// WordBoundsAt finds the boundaries of the word spanning text-space point p, if there is one.
// If there isn't, it returns an empty range whose endpoints are both equal to p.
// A word is defined as a sequence of Unicode letters and numbers, possibly with combining marks.
func (b *Buffer) WordBoundsAt(p Point) Range {
	line := b.lines[p.Y]
	i := ByteIndexForChar(line, p.X)
	startX, endX := p.X, p.X
	for j := i; j < len(line); {
		k := NextCharBoundary(line[j:])
		if !isWordChar(line[j : j+k]) {
			break
		}
		j += k
		endX++
	}
	// If we didn't find any word characters ahead of p, we're not inside a word, so we can stop now.
	if endX == p.X {
		return Range{p, p}
	}
	for lineB := []byte(line[:i]); len(lineB) > 0; {
		k := norm.NFC.LastBoundary(lineB)
		if k == -1 {
			k = 0
		}
		if !isWordChar(string(lineB[k:])) {
			break
		}
		lineB = lineB[:k]
		startX--
	}
	return Range{Point{startX, p.Y}, Point{endX, p.Y}}
}

// NextWordBoundary returns the position of the first word boundary after p. Word characters are defined as for
// WordBoundsAt.
//
// If there are no more word boundaries after p, returns p.
func (b *Buffer) NextWordBoundary(p Point) Point {
	line := b.lines[p.Y]
	i := ByteIndexForChar(line, p.X)
	q := p
	var wasInWord bool
	for j := i; j < len(line); {
		k := NextCharBoundary(line[j:])
		isInWord := isWordChar(line[j : j+k])
		if j > i && wasInWord != isInWord {
			return q
		}
		j += k
		q.X++
		wasInWord = isInWord
	}
	// If we get here, we reached the end of the line without finding a word boundary.
	if p.Y+1 < len(b.lines) {
		return Point{0, p.Y + 1}
	}
	return q
}

func isWordChar(char string) bool {
	r, _ := utf8.DecodeRuneInString(char)
	return r == '_' || unicode.In(r, unicode.L, unicode.N)
}

func ByteIndexForChar(line string, col int) int {
	p := 0
	for i := 0; p < len(line) && i < col; i++ {
		p += NextCharBoundary(line[p:])
	}
	return p
}

func (b *Buffer) Insert(text string, p Point) {
	line := b.lines[p.Y]
	insPoint := ByteIndexForChar(line, p.X)
	numNewLines := strings.Count(text, "\n")
	if numNewLines > 0 {
		b.lines = append(b.lines, make([]string, numNewLines)...)
		copy(b.lines[p.Y+1+numNewLines:], b.lines[p.Y+1:])
		nl := strings.IndexByte(text, '\n')
		carry := line[insPoint:]
		b.lines[p.Y] = line[:insPoint] + text[:nl+1]
		text = text[nl+1:]
		for i := p.Y + 1; nl != -1; i++ {
			newLine := text
			q := strings.IndexByte(text, '\n')
			if q != -1 {
				q = q + 1
				newLine, text = text[:q], text[q:]
			}
			b.lines[i] = newLine
			nl = q
		}
		b.lines[p.Y+numNewLines] = b.lines[p.Y+numNewLines] + carry
		return
	}
	b.lines[p.Y] = line[:insPoint] + text + line[insPoint:]
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

func (b *Buffer) InsertLineBreak(p Point) {
	line := b.lines[p.Y]
	b.lines = append(b.lines, "")
	copy(b.lines[p.Y+1:], b.lines[p.Y:])
	i := ByteIndexForChar(line, p.X)
	b.lines[p.Y] = line[:i] + "\n"
	b.lines[p.Y+1] = line[i:]
}

func (b *Buffer) DeleteChar(p Point) {
	// If we're deleting before the start of a line, concatenate it into the previous one,
	// then remove it.
	if p.X == 0 {
		if p.Y == 0 {
			return
		}
		prevLine := b.lines[p.Y-1]
		b.lines[p.Y-1] = prevLine[:len(prevLine)-1] + b.lines[p.Y]
		copy(b.lines[p.Y:], b.lines[p.Y+1:])
		b.lines = b.lines[:len(b.lines)-1]
	} else {
		line := b.lines[p.Y]
		i := ByteIndexForChar(line, p.X-1)
		n := NextCharBoundary(line[i:])
		b.lines[p.Y] = line[:i] + line[i+n:]
	}
}

// DeleteRange deletes all characters in the given range, including line breaks.
// The range is treated as a half-open range, and may extend past the end of the text.
func (b *Buffer) DeleteRange(r Range) {
	r = r.Normalize()
	if r.Begin.Y >= len(b.lines) {
		return
	}
	p := ByteIndexForChar(b.lines[r.Begin.Y], r.Begin.X)
	if r.End.Y >= len(b.lines) {
		b.lines[r.Begin.Y] = b.lines[r.Begin.Y][:p]
		b.lines = b.lines[r.Begin.Y+1:]
		return
	}
	q := ByteIndexForChar(b.lines[r.End.Y], r.End.X)
	b.lines[r.Begin.Y] = b.lines[r.Begin.Y][:p] + b.lines[r.End.Y][q:]
	if r.Begin.Y != r.End.Y {
		// Delete all the lines entirely between the start and the end point;
		// the line where the end point lies is deleted too, since it was
		// merged into the start line.
		copy(b.lines[r.Begin.Y+1:], b.lines[r.End.Y+1:])
		b.lines = b.lines[:len(b.lines)-(r.End.Y-r.Begin.Y)]
	}
}

// ReplaceLine replaces the contents with line y with text.
func (b *Buffer) ReplaceLine(y int, text string) { b.lines[y] = text }

// CopyRange returns a copy of the characters in the given range, as a
// contiguous slice.
func (b *Buffer) CopyRange(r Range) []byte {
	p := ByteIndexForChar(b.lines[r.Begin.Y], r.Begin.X)
	q := ByteIndexForChar(b.lines[r.End.Y], r.End.X)
	if r.Begin.Y == r.End.Y {
		return []byte(b.lines[r.Begin.Y][p:q])
	}
	out := []byte(b.lines[r.Begin.Y][p:])
	for i := r.Begin.Y + 1; i < r.End.Y; i++ {
		out = append(out, b.lines[i]...)
	}
	return append(out, b.lines[r.End.Y][:q]...)
}
