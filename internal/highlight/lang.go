package highlight

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
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
func Language(lang string, src LineSource, pal *Palette) Highlighter {
	switch lang {
	case "go":
		return &goHighlighter{src: src, palette: pal}
	default:
		// If no formatter is available for the desired language, return one
		// that doesn't do anything.
		return nullFormatter{}
	}
}

// A Palette defines the colours to be used to highlight the types of text
// recognized by the highlighter.
// Typically, Default will be left blank, to use the output device's defaults.
type Palette struct {
	Default, Comment, String Style
}

// A StyledRegion is a region of text that should be rendered with the associated style.
// The indexes reference the slice of strings that was passed to the highlighter.
type StyledRegion struct {
	Line       int
	Start, End int // Measured in bytes
	*Style
}

// A Style describes the appearance of a chunk of text.
// The zero Style means non-bold, non-underline text with the default colors
// for the output device.
type Style struct {
	Foreground, Background Color
	Bold, Underline        bool
}

// Color describes a 8-bit-per-channel RGB color.
// The zero Color is the default color for the output device.
type Color struct {
	R, G, B uint8
	Alpha   bool // Indicates that we don't want to set this color.
}

// String returns the hex color code for c.
func (c Color) String() string {
	if !c.Alpha {
		return "#DEFAULT"
	}
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
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

var (
	goLiteralStart = regexp.MustCompile("[\"'`]|/[\\*/]")
)

// Maps string delimiters to the characters to look for within the string:
// the delimiter itself or the backslash.
var goStrEvents = map[byte]string{'\'': `'\`, '"': `"\`, '`': "`"}

type goHighlighter struct {
	// state contains the formatter state at the start of each input line, except for the first;
	// the state at the first line is implicitly the zero state.
	// len(state) equals the number of lines - starting at the top - that currently have
	// highlights computed.
	state   []goHighlighterState
	regions []StyledRegion

	src     LineSource
	palette *Palette
}

type goHighlighterState struct {
	mode         int8
	strDelimiter byte
}

func (f *goHighlighter) Invalidate(ty int) {
	if ty < len(f.state) {
		f.state = f.state[:ty]
	}
	f.regions = f.regions[:regionIndexForLine(f.regions, ty)]
}

func (f *goHighlighter) Regions(startY, endY int) []StyledRegion {
	if endY > len(f.state) {
		f.run(len(f.state), f.src.SliceLines(len(f.state), endY))
	}
	return f.regions[regionIndexForLine(f.regions, startY):]
}

func (f *goHighlighter) currentState() goHighlighterState {
	if len(f.state) == 0 {
		return goHighlighterState{}
	}
	return f.state[len(f.state)-1]
}

func (f *goHighlighter) run(startY int, lines []string) {
	const (
		textNeutral = iota
		textComment
		textString
	)
	state := f.currentState()
	mode := state.mode
	strDelimiter := state.strDelimiter
	strEvents := goStrEvents[state.strDelimiter]

	var line string
	for i, j := 0, 0; j < len(lines); {
		line = lines[j]
		if i >= len(line) {
			f.state = append(f.state, goHighlighterState{mode, strDelimiter})
			j++
			i = 0
			continue
		}
		// Compute the actual Y coordinate of the line in the source text to
		// correctly annotate the regions.
		ty := startY + j
		switch mode {
		case textNeutral:
			next := goLiteralStart.FindStringIndex(line[i:])
			if next == nil {
				i = len(line)
				continue
			}
			switch line[i+next[0] : i+next[1]] {
			case `"`, "'", "`":
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i + next[0], End: i + next[1], Style: &f.palette.String})
				mode = textString
				strDelimiter = line[i+next[0]]
				strEvents = goStrEvents[strDelimiter]
				i += next[1]
			case "//":
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i + next[0], End: len(line), Style: &f.palette.Comment})
				i = len(line)
			case "/*":
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i + next[0], End: i + next[1], Style: &f.palette.Comment})
				mode = textComment
				i += next[1]
			}
		case textComment:
			if next := strings.Index(line[i:], "*/"); next == -1 {
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i, End: len(line), Style: &f.palette.Comment})
				i = len(line)
			} else {
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i, End: i + next + 2, Style: &f.palette.Comment})
				mode = textNeutral
				i += next + 2
			}
		case textString:
			if next := strings.IndexAny(line[i:], strEvents); next != -1 {
				switch line[i+next] {
				// If we find an escaped anything - including, in particular, a quote -
				// skip over it. Some escape sequences are longer than 2 characters, but
				// none of them are supposed to contain quotes, so this shortcut is OK.
				case '\\':
					f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i, End: i + next + 2, Style: &f.palette.String})
					i += next + 2
				default:
					f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i, End: i + next + 1, Style: &f.palette.String})
					mode = textNeutral
					i += next + 1
				}
			} else {
				if strDelimiter != '`' {
					mode = textNeutral
				}
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i, End: len(line), Style: &f.palette.String})
				i = len(line)
			}
		}
	}
	return
}
