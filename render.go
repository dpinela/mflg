package main

import (
	"strconv"
	"strings"

	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/color"
	"github.com/dpinela/mflg/internal/config"
	"github.com/dpinela/mflg/internal/highlight"
	"github.com/dpinela/mflg/internal/termdraw"
	"github.com/dpinela/mflg/internal/termesc"

	"github.com/dpinela/charseg"
	"github.com/mattn/go-runewidth"
)

func (w *window) redraw(console *termdraw.Screen) { w.redrawAtYOffset(console, 0) }

// redrawAtYOffset renders the window's contents onto a console.
func (w *window) redrawAtYOffset(console *termdraw.Screen, yOffset int) {
	lines := w.wrappedBuf.Lines(w.topLine, w.topLine+w.height)
	var hr []highlight.StyledRegion
	if len(lines) != 0 {
		hr = w.highlighter.Regions(lines[0].Start.Y, lines[len(lines)-1].Start.Y+1)
	}

	tf := textFormatter{src: lines, highlightedRegions: hr,
		invertedRegion: w.selection, gutterWidth: w.gutterWidth(), gutterText: w.customGutterText, config: w.app.config}
	n := min(w.height, len(lines))
	for j := 0; j < n; j++ {
		tf.formatLine(console, yOffset, j)
	}
}

type textFormatter struct {
	src                []buffer.WrappedLine
	currentHighlight   *highlight.StyledRegion
	highlightedRegions []highlight.StyledRegion
	invertedRegion     optionalTextRange
	gutterText         string
	gutterWidth        int
	config             *config.Config
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

var numericGutterStyle = termdraw.Style{Foreground: &color.Color{R: 200, G: 200, B: 200}}

func (tf *textFormatter) formatLine(console *termdraw.Screen, yOffset, j int) {
	line := strings.TrimSuffix(tf.src[j].Text, "\n")
	tp := tf.src[j].Start
	bx := tf.src[j].ByteStart
	wp := termdraw.Point{X: 0, Y: yOffset + j}
	gutterStyle := termdraw.Style{Bold: true}
	gutterText := tf.gutterText
	if gutterText == "" {
		gutterText = strconv.Itoa(tp.Y + 1)
		gutterStyle = numericGutterStyle
	}
	for gutterText != "" {
		c := charseg.FirstGraphemeCluster(gutterText)
		console.Put(wp, termdraw.Cell{Content: c, Style: gutterStyle})
		wp.X += runewidth.StringWidth(c)
		gutterText = gutterText[len(c):]
	}
	for wp.X < tf.gutterWidth {
		console.Put(wp, termdraw.Cell{})
		wp.X++
	}
	style := termdraw.Style{}
	if tf.invertedRegion.Set && !tp.Less(tf.invertedRegion.Begin) && tp.Less(tf.invertedRegion.End) {
		style.Inverted = true
	}
	if tf.currentHighlight != nil && tp.Y == tf.currentHighlight.Line && bx >= tf.currentHighlight.Start && bx < tf.currentHighlight.End {
		mergeStyle(&style, tf.style())
	}
	for len(line) > 0 {
		if tf.invertedRegion.Set {
			switch tp {
			case tf.invertedRegion.Begin:
				style.Inverted = true
			case tf.invertedRegion.End:
				style.Inverted = false
			}
		}
		if tf.currentHighlight != nil && (tp.Y > tf.currentHighlight.Line || bx >= tf.currentHighlight.End) {
			tf.currentHighlight = nil
			style = termdraw.Style{Inverted: style.Inverted} // reset other attributes
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
					mergeStyle(&style, tf.style())
					break
				}
			}
		}
		n := buffer.NextCharBoundary(line)
		switch {
		case line[:n] == "\t":
			for i := 0; i < tf.config.TabWidth; i++ {
				console.Put(wp, termdraw.Cell{Style: style})
				wp.X++
			}
		case line[:n] == "\n":
		case n == 1 && line[0] < ' ':
			console.Put(wp, termdraw.Cell{Content: string('\u2400' + rune(line[0])), Style: style})
			wp.X++
		case line[:n] == "\x7f":
			console.Put(wp, termdraw.Cell{Content: "\u2421", Style: style})
			wp.X++
		default:
			console.Put(wp, termdraw.Cell{Content: line[:n], Style: style})
			wp.X += runewidth.StringWidth(line[:n])
		}
		bx += n
		line = line[n:]
		tp.X++
	}
}

func mergeStyle(ts *termdraw.Style, cs config.Style) {
	ts.Foreground = cs.Foreground
	ts.Background = cs.Background
	ts.Bold = cs.Bold
	ts.Italic = cs.Italic
	ts.Underline = cs.Underline
}

func (tf *textFormatter) style() config.Style {
	switch tf.currentHighlight.Style {
	case highlight.StyleComment:
		return tf.config.TextStyle.Comment
	case highlight.StyleString:
		return tf.config.TextStyle.String
	default:
		return config.Style{}
	}
}

func appendSpaces(b []byte, n int) []byte {
	for i := 0; i < n; i++ {
		b = append(b, ' ')
	}
	return b
}
