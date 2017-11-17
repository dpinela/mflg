package main

import (
	"io/ioutil"
	"strings"
	"testing"
)

func BenchmarkRedraw(b *testing.B) {
	w := newTestWindow(b, 100, 30, strings.Repeat(testDocument, 20))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w.needsRedraw = true
		w.redraw(ioutil.Discard)
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
