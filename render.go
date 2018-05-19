package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/highlight"
	"github.com/dpinela/mflg/internal/termesc"

	"github.com/mattn/go-runewidth"
)

func (w *window) redraw(console io.Writer) error { return w.redrawAtYOffset(console, 0) }

// redrawAtYOffset renders the window's contents onto a console.
// If the console is nil, it only updates the window's layout.
func (w *window) redrawAtYOffset(console io.Writer, yOffset int) error {
	if !w.needsRedraw {
		return nil
	}
	if _, err := fmt.Fprint(console, termesc.SetCursorPos(yOffset+1, 1), termesc.ClearScreenForward); err != nil {
		return err
	}
	lines := w.wrappedBuf.Lines(w.topLine, w.topLine+w.height)
	var hr []highlight.StyledRegion
	if len(lines) != 0 {
		hr = w.highlighter.Regions(lines[0].Start.Y, lines[len(lines)-1].Start.Y+1)
	}
	tf := textFormatter{src: lines, highlightedRegions: hr,
		invertedRegion: w.selection, gutterWidth: w.gutterWidth(), gutterText: w.customGutterText, tabWidth: w.getTabWidth()}
	for wy := 0; wy < w.height; wy++ {
		line, ok := tf.formatNextLine(wy+1 >= w.height)
		if !ok {
			break
		}
		if _, err := console.Write(line); err != nil {
			return err
		}
	}
	w.needsRedraw = console == nil
	return nil
}

type textFormatter struct {
	src                []buffer.WrappedLine
	currentHighlight   *highlight.StyledRegion
	highlightedRegions []highlight.StyledRegion
	invertedRegion     optionalTextRange
	gutterText         string
	gutterWidth        int
	tabWidth           int

	line int
	buf  []byte
}

// Pre-compute the SGR escape sequences used in formatNextLine to avoid the expense of recomputing them repeatedly.
var (
	styleInverted     = termesc.SetGraphicAttributes(termesc.StyleInverted)
	styleNotInverted  = termesc.SetGraphicAttributes(termesc.StyleNotInverted)
	styleResetToBold  = termesc.SetGraphicAttributes(termesc.StyleNone, termesc.StyleBold)
	styleResetToWhite = termesc.SetGraphicAttributes(termesc.StyleNone, termesc.ColorWhite)
	styleReset        = termesc.SetGraphicAttributes(termesc.StyleNone)
	styleResetColor   = termesc.SetGraphicAttributes(termesc.ColorDefault)
)

func (tf *textFormatter) formatNextLine(last bool) ([]byte, bool) {
	if tf.line >= len(tf.src) {
		return nil, false
	}
	line := strings.TrimSuffix(tf.src[tf.line].Text, "\n")
	tp := tf.src[tf.line].Start
	bx := tf.src[tf.line].ByteStart
	var gutterLen int
	if tf.gutterText != "" {
		tf.buf = append(tf.buf[:0], styleResetToBold...)
		gutterLen = runewidth.StringWidth(tf.gutterText)
		tf.buf = append(tf.buf, tf.gutterText...)
	} else {
		tf.buf = append(tf.buf[:0], styleResetToWhite...)
		n := len(tf.buf)
		tf.buf = strconv.AppendInt(tf.buf, int64(tp.Y)+1, 10)
		gutterLen = len(tf.buf) - n
	}
	tf.buf = append(tf.buf, styleReset...)
	for i := gutterLen; i < tf.gutterWidth; i++ {
		tf.buf = append(tf.buf, ' ')
	}
	if tf.invertedRegion.Set && !tp.Less(tf.invertedRegion.Begin) && tp.Less(tf.invertedRegion.End) {
		tf.buf = append(tf.buf, styleInverted...)
	}
	if tf.currentHighlight != nil && tp.Y == tf.currentHighlight.Line && bx >= tf.currentHighlight.Start && bx < tf.currentHighlight.End {
		tf.buf = append(tf.buf, makeSGRString(tf.currentHighlight.Style)...)
	}
	for len(line) > 0 {
		if tf.invertedRegion.Set {
			switch tp {
			case tf.invertedRegion.Begin:
				tf.buf = append(tf.buf, styleInverted...)
			case tf.invertedRegion.End:
				tf.buf = append(tf.buf, styleNotInverted...)
			}
		}
		if tf.currentHighlight != nil && (tp.Y > tf.currentHighlight.Line || bx >= tf.currentHighlight.End) {
			tf.currentHighlight = nil
			tf.buf = append(tf.buf, styleResetColor...)
		}
		if tf.currentHighlight == nil {
			// Find the next highlighted region that covers the current point.
			// Break early to avoid wasting time with ones that can't possibly apply.
			// TODO: make this search more efficient.
			for i, r := range tf.highlightedRegions {
				if tp.Y < r.Line || (tp.Y == r.Line && bx < r.Start) {
					break
				}
				if tp.Y == r.Line && bx >= r.Start && bx < r.End {
					tf.currentHighlight = &tf.highlightedRegions[i]
					tf.highlightedRegions = tf.highlightedRegions[i+1:]
					tf.buf = append(tf.buf, makeSGRString(tf.currentHighlight.Style)...)
					break
				}
			}
		}
		n := buffer.NextCharBoundary(line)
		switch {
		case line[:n] == "\t":
			tf.appendSpaces(tf.tabWidth)
		case line[:n] == "\n":
		case n == 1 && line[0] < ' ':
			tf.buf = append(tf.buf, string('\u2400'+rune(line[0]))...)
		case line[:n] == "\x7f":
			tf.buf = append(tf.buf, "\u2421"...)
		default:
			tf.buf = append(tf.buf, line[:n]...)
		}
		bx += n
		line = line[n:]
		tp.X++
	}
	tf.buf = append(tf.buf, styleReset...)
	if !last {
		tf.buf = append(tf.buf, '\r', '\n')
	}
	tf.line++
	return tf.buf, true
}

func (tf *textFormatter) appendSpaces(n int) {
	for i := 0; i < n; i++ {
		tf.buf = append(tf.buf, ' ')
	}
}

func makeSGRString(s *highlight.Style) string {
	var params []termesc.GraphicAttribute
	if fg := s.Foreground; fg.Alpha {
		params = append(params, termesc.OutputColor(fg.Color))
	} else {
		params = append(params, termesc.ColorDefault)
	}
	if bg := s.Background; bg.Alpha {
		params = append(params, termesc.OutputColorBackground(bg.Color))
	} else {
		params = append(params, termesc.ColorDefaultBackground)
	}
	if s.Bold {
		params = append(params, termesc.StyleBold)
	} else {
		params = append(params, termesc.StyleNotBold)
	}
	if s.Underline {
		params = append(params, termesc.StyleUnderline)
	} else {
		params = append(params, termesc.StyleNotUnderline)
	}
	return termesc.SetGraphicAttributes(params...)
}
