package highlight

import (
	"regexp"
	"strings"
)

var (
	goLiteralStart = regexp.MustCompile("[\"'`]|/[\\*/]")
	goStrEvents    = map[byte]string{'\'': `'\`, '"': `"\`, '`': "`", '/': `/\`}

	cLiteralStart = regexp.MustCompile(`["']|/[\*/]`)

	jsonLiteralStart = regexp.MustCompile(`"`)
)

type cStyleHighlighter struct {
	// state contains the formatter state at the start of each input line, except for the first;
	// the state at the first line is implicitly the zero state.
	// len(state) equals the number of lines - starting at the top - that currently have
	// highlights computed.
	state   []cStyleHighlighterState
	regions []StyledRegion

	// Maps string delimiters to the characters to look for within the string:
	// the delimiter itself or the backslash.
	strEvents    map[byte]string
	literalStart *regexp.Regexp

	src LineSource
}

type cStyleHighlighterState struct {
	mode         int8
	strDelimiter byte
}

func (f *cStyleHighlighter) Invalidate(ty int) {
	if ty < len(f.state) {
		f.state = f.state[:ty]
	}
	f.regions = f.regions[:regionIndexForLine(f.regions, ty)]
}

func (f *cStyleHighlighter) Regions(startY, endY int) []StyledRegion {
	if endY > len(f.state) {
		f.run(len(f.state), f.src.SliceLines(len(f.state), endY))
	}
	return f.regions[regionIndexForLine(f.regions, startY):]
}

func (f *cStyleHighlighter) currentState() cStyleHighlighterState {
	if len(f.state) == 0 {
		return cStyleHighlighterState{}
	}
	return f.state[len(f.state)-1]
}

func (f *cStyleHighlighter) run(startY int, lines []string) {
	const (
		textNeutral = iota
		textComment
		textString
	)
	state := f.currentState()
	mode := state.mode
	strDelimiter := state.strDelimiter
	strEvents := f.strEvents[state.strDelimiter]

	var line string
	for i, j := 0, 0; j < len(lines); {
		line = lines[j]
		if i >= len(line) {
			f.state = append(f.state, cStyleHighlighterState{mode, strDelimiter})
			j++
			i = 0
			continue
		}
		// Compute the actual Y coordinate of the line in the source text to
		// correctly annotate the regions.
		ty := startY + j
		switch mode {
		case textNeutral:
			next := f.literalStart.FindStringIndex(line[i:])
			if next == nil {
				i = len(line)
				continue
			}
			switch line[i+next[0] : i+next[1]] {
			case `"`, "'", "`":
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i + next[0], End: i + next[1], Style: StyleString})
				mode = textString
				strDelimiter = line[i+next[0]]
				strEvents = f.strEvents[strDelimiter]
				i += next[1]
			case "//":
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i + next[0], End: len(line), Style: StyleComment})
				i = len(line)
			case "/*":
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i + next[0], End: i + next[1], Style: StyleComment})
				mode = textComment
				i += next[1]
			}
		case textComment:
			if next := strings.Index(line[i:], "*/"); next == -1 {
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i, End: len(line), Style: StyleComment})
				i = len(line)
			} else {
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i, End: i + next + 2, Style: StyleComment})
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
					f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i, End: i + next + 2, Style: StyleString})
					i += next + 2
				default:
					f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i, End: i + next + 1, Style: StyleString})
					mode = textNeutral
					i += next + 1
				}
			} else {
				if strDelimiter != '`' {
					mode = textNeutral
				}
				f.regions = appendRegion(f.regions, StyledRegion{Line: ty, Start: i, End: len(line), Style: StyleString})
				i = len(line)
			}
		}
	}
	return
}
