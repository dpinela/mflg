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
	buf := w.drawBuffer
	n := min(w.height, len(lines))
	for wy := 0; wy < n; wy++ {
		buf = tf.formatLine(buf, wy, wy+1 >= w.height)
	}
	w.drawBuffer = buf[:0] // allow this buffer to be reused next time
	if _, err := console.Write(buf); err != nil {
		return err
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
}

// Pre-compute the SGR escape sequences used in formatNextLine to avoid the expense of recomputing them repeatedly.
var (
	styleInverted     = termesc.SetGraphicAttributes(termesc.StyleInverted)
	styleNotInverted  = termesc.SetGraphicAttributes(termesc.StyleNotInverted)
	styleResetToBold  = termesc.SetGraphicAttributes(termesc.StyleNone, termesc.StyleBold)
	styleResetToWhite = termesc.SetGraphicAttributes(termesc.StyleNone, termesc.ColorWhite)
	styleReset        = termesc.SetGraphicAttributes(termesc.StyleNone)
	styleResetColor   = termesc.SetGraphicAttributes(termesc.ColorDefault, termesc.ColorDefaultBackground, termesc.StyleNotBold, termesc.StyleNotItalic, termesc.StyleNotUnderline)
)

func (tf *textFormatter) formatLine(buf []byte, wy int, last bool) []byte {
	line := strings.TrimSuffix(tf.src[wy].Text, "\n")
	tp := tf.src[wy].Start
	bx := tf.src[wy].ByteStart
	var gutterLen int
	if tf.gutterText != "" {
		buf = append(buf, styleResetToBold...)
		gutterLen = runewidth.StringWidth(tf.gutterText)
		buf = append(buf, tf.gutterText...)
	} else {
		buf = append(buf, styleResetToWhite...)
		n := len(buf)
		buf = strconv.AppendInt(buf, int64(tp.Y)+1, 10)
		gutterLen = len(buf) - n
	}
	buf = append(buf, styleReset...)
	for i := gutterLen; i < tf.gutterWidth; i++ {
		buf = append(buf, ' ')
	}
	if tf.invertedRegion.Set && !tp.Less(tf.invertedRegion.Begin) && tp.Less(tf.invertedRegion.End) {
		buf = append(buf, styleInverted...)
	}
	if tf.currentHighlight != nil && tp.Y == tf.currentHighlight.Line && bx >= tf.currentHighlight.Start && bx < tf.currentHighlight.End {
		buf = append(buf, makeSGRString(tf.currentHighlight.Style)...)
	}
	for len(line) > 0 {
		if tf.invertedRegion.Set {
			switch tp {
			case tf.invertedRegion.Begin:
				buf = append(buf, styleInverted...)
			case tf.invertedRegion.End:
				buf = append(buf, styleNotInverted...)
			}
		}
		if tf.currentHighlight != nil && (tp.Y > tf.currentHighlight.Line || bx >= tf.currentHighlight.End) {
			tf.currentHighlight = nil
			buf = append(buf, styleResetColor...)
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
					buf = append(buf, makeSGRString(tf.currentHighlight.Style)...)
					break
				}
			}
		}
		n := buffer.NextCharBoundary(line)
		switch {
		case line[:n] == "\t":
			buf = appendSpaces(buf, tf.tabWidth)
		case line[:n] == "\n":
		case n == 1 && line[0] < ' ':
			buf = append(buf, string('\u2400'+rune(line[0]))...)
		case line[:n] == "\x7f":
			buf = append(buf, "\u2421"...)
		default:
			buf = append(buf, line[:n]...)
		}
		bx += n
		line = line[n:]
		tp.X++
	}
	buf = append(buf, styleReset...)
	if !last {
		buf = append(buf, '\r', '\n')
	}
	return buf
}

func appendSpaces(b []byte, n int) []byte {
	for i := 0; i < n; i++ {
		b = append(b, ' ')
	}
	return b
}

func makeSGRString(s *highlight.Style) string {
	var params []termesc.GraphicAttribute
	// At the end of each highlighted region, these flags are all reset,
	// so at the start of this one we know that they're all off.
	if fg := s.Foreground; fg != nil {
		params = append(params, termesc.OutputColor(*fg))
	}
	if bg := s.Background; bg != nil {
		params = append(params, termesc.OutputColorBackground(*bg))
	}
	if s.Bold {
		params = append(params, termesc.StyleBold)
	}
	if s.Italic {
		params = append(params, termesc.StyleItalic)
	}
	if s.Underline {
		params = append(params, termesc.StyleUnderline)
	}
	return termesc.SetGraphicAttributes(params...)
}
