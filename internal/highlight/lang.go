package highlight

import (
	"sort"
)

// A Highlighter provides syntax highlighting for a specific language.
type Highlighter interface {
	// Invalidate notifies the highlighter that the source text starting at line ty
	// has changed.
	Invalidate(ty int)
	// Regions returns all highlighted regions belonging to lines in the interval
	// [startY, endY[. It may also return additional regions past the end of that interval.
	// Callers should not modify the returned slice.
	Regions(startY, endY int) []StyledRegion
}

// LineSource is the interface used to fetch lines to be highlighted.
// It is implemented by *buffer.Buffer.
type LineSource interface {
	SliceLines(i, j int) []string
}

// Language returns a Highlighter appropriate for the specified language.
// The styles returned by Regions can point to fields of the given palette;
// modifying the palette will change these styles automatically.
// It always returns a non-nil Highlighter.
func Language(lang string, src LineSource) Highlighter {
	switch lang {
	case "go":
		return &cStyleHighlighter{src: src, strEvents: goStrEvents, literalStart: goLiteralStart}
	case "c", "java":
		return &cStyleHighlighter{src: src, strEvents: goStrEvents, literalStart: cLiteralStart}
	case "json":
		return &cStyleHighlighter{src: src, strEvents: goStrEvents, literalStart: jsonLiteralStart}
	default:
		// If no formatter is available for the desired language, return one
		// that doesn't do anything.
		return nullFormatter{}
	}
}

// A Style indicates which text style should be used for a Region.
type Style int

const (
	StyleNone Style = iota
	StyleComment
	StyleString
)

// A StyledRegion is a region of text that should be rendered with the associated style.
// The indexes reference the slice of strings that was passed to the highlighter.
type StyledRegion struct {
	Line       int
	Start, End int // Measured in bytes
	Style
}

// appendRegion appends r to out, coalescing it with the last region in out
// if they're adjacent. It returns the extended slice, just like append.
func appendRegion(out []StyledRegion, r StyledRegion) []StyledRegion {
	if r.Start == r.End {
		return out
	}
	if n := len(out); n != 0 && out[n-1].Line == r.Line && out[n-1].End == r.Start {
		out[n-1].End = r.End
		return out
	}
	return append(out, r)
}

// regionIndexForLine returns the index of the first region in rs whose line >= ty, or
// len(rs) if no such region exists.
func regionIndexForLine(rs []StyledRegion, ty int) int {
	return sort.Search(len(rs), func(j int) bool { return rs[j].Line >= ty })
}

type nullFormatter struct{}

func (nullFormatter) Invalidate(int)                  {}
func (nullFormatter) Regions(int, int) []StyledRegion { return nil }
