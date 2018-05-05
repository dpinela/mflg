package buffer

import (
	"sort"
	"strings"
)

// Coordinate spaces:
// - Window space - measured in terminal cells, (0, 0) is the top-left corner of the file; used for marking the current scroll and cursor position
// - Viewport space - measured in terminal cells, (0, 0) is the top-left corner of the window; input and output coordinates must be in this space
// - Text space - measured in characters (as defined by buffer.NextCharBoundary), (0, 0) is the first character of the file; used for non-visual references into the text, such as the selection, input to the implementation of all editing commands, and (eventually) navigation points.

// Conversions:
// - Window space <=> text space - requires scanning the text to find the matching position
//   (needed when any editing command is used)
// - Window space <=> viewport space - requires only an addition/subtraction;
//   (window coords) = (viewport coords) + (window coords of top left corner of viewport)

// WrappedLine is a segment of a line of text that fits in one viewport line,
// annotated with the text space coordinates of its first character.
type WrappedLine struct {
	Start     Point
	ByteStart int // The byte index of Start within the original line
	Text      string
}

// Point represents a two-dimensional integer point in text or window space.
type Point struct{ X, Y int }

// Less reports whether p comes before q in the text.
func (p Point) Less(q Point) bool {
	if p.Y < q.Y {
		return true
	}
	if p.Y > q.Y {
		return false
	}
	return p.X < q.X
}

// Range represents the region of text lying between two Points, as defined by Point.Less.
// For any valid Range r, r.End.Less(r.Begin) is false.
type Range struct{ Begin, End Point }

// Normalize returns r with its endpoints swapped if necessary so that it is valid.
func (r Range) Normalize() Range {
	if r.End.Less(r.Begin) {
		return Range{r.End, r.Begin}
	}
	return r
}

// Empty reports whether r spans no characters.
func (r Range) Empty() bool { return r.Begin == r.End }

// WrappedBuffer manages line wrapping for a Buffer.
//
// When using a WrappedBuffer for display, all edits should go through it to ensure that the wrap boundaries
// are updated accordingly, using its Insert, InsertLineBreak, DeleteRange and DeleteChar methods.
type WrappedBuffer struct {
	lineWidth, tabWidth int
	displayWidth        func(string) int // A function that returns the display width of a string.
	lines               []WrappedLine
	src                 *Buffer
}

// NewWrapped creates a WrappedBuffer which gets the original text from src, and sets its line width.
// displayWidth should return the number of terminal cells occupied by a character.
func NewWrapped(src *Buffer, width int, displayWidth func(string) int) *WrappedBuffer {
	return &WrappedBuffer{
		lineWidth:    width,
		displayWidth: displayWidth,
		tabWidth:     displayWidth("\t"),
		src:          src,
	}
}

// HasLines reports whether line y is within the bounds of window space (that is, if there are at least
// y + 1 wrapped lines).
func (wb *WrappedBuffer) HasLine(y int) bool {
	wb.wrapUntil(y)
	return y < len(wb.lines)
}

// Lines returns the wrapped lines in the interval [begin, end[ in window space.
// If begin or end exceed the limit of window space, this may return fewer than end - begin lines.
func (wb *WrappedBuffer) Lines(begin, end int) []WrappedLine {
	wb.wrapUntil(end)
	if end >= len(wb.lines) {
		end = len(wb.lines)
	}
	if begin > end {
		begin = end
	}
	return wb.lines[begin:end]
}

// IndexTextPos finds the Y coordinate where the window space point matching tp lies.
func (wb *WrappedBuffer) WindowYForTextPos(tp Point) int {
	wy := 0
	// Relies on the Line method to populate wb.lines
	for wb.Line(wy).Start.Less(tp) && wy < len(wb.lines) {
		wy = wy*2 + 1
	}
	if wy > len(wb.lines) {
		wy = len(wb.lines)
	}
	wy = sort.Search(wy, func(i int) bool { return !wb.lines[i].Start.Less(tp) })
	if wy < len(wb.lines) && wb.lines[wy].Start == tp {
		return wy
	}
	if wy < 1 {
		panic("buffer: found window point before (0, 0)")
	}
	return wy - 1
}

// Line returns the specified line in window space, or the last line if wy is below its end.
func (wb *WrappedBuffer) Line(wy int) WrappedLine {
	wb.wrapUntil(wy)
	if wy >= len(wb.lines) {
		wy = len(wb.lines) - 1
	}
	return wb.lines[wy]
}

// Reset replaces the underlying Buffer and forces all lines to be re-wrapped.
func (wb *WrappedBuffer) Reset(b *Buffer) {
	wb.src = b
	wb.refresh()
}

// Insert inserts text into the underlying Buffer.
func (wb *WrappedBuffer) Insert(text string, tp Point) {
	wb.src.Insert(text, tp)
	wb.refreshFrom(tp.Y)
}

// InsertLineBreak inserts a line break into the underlying Buffer.
func (wb *WrappedBuffer) InsertLineBreak(tp Point) {
	wb.src.InsertLineBreak(tp)
	wb.refreshFrom(tp.Y)
}

// DeleteRange deletes all characters in the underlying Buffer within the specified range.
func (wb *WrappedBuffer) DeleteRange(tr Range) {
	wb.src.DeleteRange(tr)
	wb.refreshFrom(tr.Begin.Y)
}

// DeleteChar deletes the character preceding the one at (ty, tx).
func (wb *WrappedBuffer) DeleteChar(tp Point) {
	wb.src.DeleteChar(tp)
	if tp.X == 0 && tp.Y > 0 {
		wb.refreshFrom(tp.Y - 1)
	} else {
		wb.refreshFrom(tp.Y)
	}
}

// ReplaceLine replaces the entire content of line ty with text.
func (wb *WrappedBuffer) ReplaceLine(ty int, text string) {
	wb.src.ReplaceLine(ty, text)
	wb.refreshFrom(ty)
}

// SetWidth changes the line width of the buffer.
func (wb *WrappedBuffer) SetWidth(newWidth int) {
	if newWidth != wb.lineWidth {
		wb.lineWidth = newWidth
		wb.refresh()
	}
}

// refresh invalidates all existing wrapped lines; it should be used when the source buffer is updated or when
// the wrap width is changed.
func (wb *WrappedBuffer) refresh() { wb.lines = wb.lines[:0] }

// refreshFrom invalidates all wrapped lines from the given text Y coordinate onwards.
func (wb *WrappedBuffer) refreshFrom(ty int) {
	wy := sort.Search(len(wb.lines), func(wy int) bool { return !wb.lines[wy].Start.Less(Point{0, ty}) })
	wb.lines = wb.lines[:wy]
}

// wrapUntil wraps the source buffer until the end of wrapped line i.
func (wb *WrappedBuffer) wrapUntil(i int) {
	// Save the work of re-wrapping lines that are already wrapped.
	srcStart, wrappedStart := wb.lastWrappedSrcLine()
	wb.lines = wb.lines[:wrappedStart]
	for j := srcStart; j < wb.src.LineCount() && len(wb.lines) <= i; j++ {
		srcLine := wb.src.Line(j)
		// For almost any character, the number of cells it occupies is not greater than its length in
		// UTF-8 bytes (any double-width chars take at least 2 bytes). The exception is the tab character,
		// which takes 1 byte but is usually rendered as several spaces. Thus, if the byte length of a
		// line, adjusted for tabs, is shorter than the viewport width, then so is the visual width, and
		// we don't need to wrap that line.
		if len(srcLine)+strings.Count(srcLine, "\t")*(wb.tabWidth-1) <= wb.lineWidth {
			wb.lines = append(wb.lines, WrappedLine{Point{0, j}, 0, srcLine})
			continue
		}
		k := 0
		wx := 0
		tx := 0
		lastStartByte := 0
		lastStartTX := 0
		for k < len(srcLine) && len(wb.lines) <= i {
			c := srcLine[k : k+NextCharBoundary(srcLine[k:])]
			if c == "\n" {
				break
			}
			w := wb.displayWidth(c)
			if wx+w > wb.lineWidth && wx > 0 {
				wb.lines = append(wb.lines, WrappedLine{Point{lastStartTX, j}, lastStartByte, srcLine[lastStartByte:k]})
				lastStartByte = k
				lastStartTX = tx
				wx = 0
			} else {
				k += len(c)
				wx += w
				tx++
			}
		}
		if len(wb.lines) <= i {
			wb.lines = append(wb.lines, WrappedLine{Point{lastStartTX, j}, lastStartByte, srcLine[lastStartByte:]})
		}
	}
}

func (wb *WrappedBuffer) lastWrappedSrcLine() (srcIndex, wrappedIndex int) {
	for i := len(wb.lines) - 1; i >= 0; i-- {
		if wb.lines[i].Start.Y != wb.lines[len(wb.lines)-1].Start.Y {
			return wb.lines[i+1].Start.Y, i + 1
		}
	}
	return 0, 0
}
