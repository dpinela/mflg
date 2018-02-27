package main

import (
	"testing"
)

// This file contains tests for behaviour that was at some point found to crash mflg.

func TestUndoPastSelectionBound(t *testing.T) { // Discovered by fuzzing
	w := newTestWindow(t, 80, 25, "")
	w.typeText("\r")
	w.markSelectionBound()
	w.undo()
	w.markSelectionBound()
	w.typeText("\r")
	checkLineContent(t, 1, w, 0, "")
	checkLineContent(t, 1, w, 1, "")
	checkCursorPos(t, 1, w, point{0, 1})
}
