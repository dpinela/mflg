package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/dpinela/mflg/internal/buffer"
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
	tf := textFormatter{src: lines,
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
	src            []buffer.WrappedLine
	invertedRegion optionalTextRange
	gutterText     string
	gutterWidth    int
	tabWidth       int

	line int
	buf  []byte
}

// Pre-compute the SGR escape sequences used in formatNextLine to avoid the expense of recomputing them repeatedly.
var (
	styleInverted     = termesc.SetGraphicAttributes(termesc.StyleInverted)
	styleResetToBold  = termesc.SetGraphicAttributes(termesc.StyleNone, termesc.StyleBold)
	styleResetToWhite = termesc.SetGraphicAttributes(termesc.StyleNone, termesc.ColorWhite)
	styleReset        = termesc.SetGraphicAttributes(termesc.StyleNone)
)

func (tf *textFormatter) formatNextLine(last bool) ([]byte, bool) {
	if tf.line >= len(tf.src) {
		return nil, false
	}
	line := strings.TrimSuffix(tf.src[tf.line].Text, "\n")
	tp := tf.src[tf.line].Start
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
	for len(line) > 0 {
		if tf.invertedRegion.Set {
			switch tp {
			case tf.invertedRegion.Begin:
				tf.buf = append(tf.buf, styleInverted...)
			case tf.invertedRegion.End:
				tf.buf = append(tf.buf, styleReset...)
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
		line = line[n:]
		tp.X++
	}
	if tf.invertedRegion.Set && ((tp.Y >= tf.invertedRegion.Begin.Y && tp.Y < tf.invertedRegion.End.Y) || tf.invertedRegion.End == tp) {
		tf.buf = append(tf.buf, styleReset...)
	}
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
