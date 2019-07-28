package main

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/dpinela/mflg/internal/termdraw"
)

func BenchmarkRedraw(b *testing.B) {
	w := newTestWindow(b, 100, 30, strings.Repeat(testDocument, 20))
	s := termdraw.NewScreen(ioutil.Discard, termdraw.Point{X: 100, Y: 30})
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.needsRedraw = true
		w.redraw(s)
		s.Flip()
	}
}

func BenchmarkMoveCursor(b *testing.B) {
	w := newTestWindow(b, 100, 30, testDocument)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%80 == 0 {
			w.cursorPos = point{}
		}
		w.moveCursorRight()
	}
}
