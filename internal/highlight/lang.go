package highlight

import (
	"fmt"
	"regexp"
	"strings"
)

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

func (c Color) String() string {
	if !c.Alpha {
		return "#DEFAULT"
	}
	return fmt.Sprintf("#%02X%02X%02X", c.R, c.G, c.B)
}

var (
	goLiteralStart = regexp.MustCompile("[\"'`]|/[\\*/]")
)

func lexGo(lines []string, pal *Palette) (out []StyledRegion) {
	const (
		textNeutral = iota
		textComment
		textString
	)
	state := textNeutral
	var strDelimiter byte

	var line string
	for i, j := 0, 0; j < len(lines); {
		line = lines[j]
		if i == len(line) {
			j++
			i = 0
			continue
		}
		switch state {
		case textNeutral:
			next := goLiteralStart.FindStringIndex(line[i:])
			if next == nil {
				i = len(line)
				continue
			}
			switch line[i+next[0] : i+next[1]] {
			case `"`, "'", "`":
				out = append(out, StyledRegion{Line: j, Start: i + next[0], End: i + next[1], Style: &pal.String})
				state = textString
				strDelimiter = line[i+next[0]]
				i += next[1]
			case "//":
				out = append(out, StyledRegion{Line: j, Start: i + next[0], End: len(line), Style: &pal.Comment})
				i = len(line)
			case "/*":
				out = append(out, StyledRegion{Line: j, Start: i + next[0], End: i + next[1], Style: &pal.Comment})
				state = textComment
				i += next[1]
			}
		case textComment:
			if next := strings.Index(line[i:], "*/"); next == -1 {
				out = append(out, StyledRegion{Line: j, Start: i, End: len(line), Style: &pal.Comment})
				i = len(line)
			} else {
				out = append(out, StyledRegion{Line: j, Start: i, End: i + next + 2, Style: &pal.Comment})
				state = textNeutral
				i += next + 2
			}
		case textString:
			if next := strings.IndexByte(line[i:], strDelimiter); next != -1 {
				out = append(out, StyledRegion{Line: j, Start: i, End: i + next + 1, Style: &pal.String})
				if !(strDelimiter != '`' && next > 0 && line[i+next-1] == '\\') {
					state = textNeutral
				}
				i += next + 1
			} else {
				if strDelimiter != '`' {
					state = textNeutral
				}
				out = append(out, StyledRegion{Line: j, Start: i, End: len(line), Style: &pal.String})
				i = len(line)
			}
		}
	}
	return
}
