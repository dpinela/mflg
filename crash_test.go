package main

import (
	"testing"

	"github.com/dpinela/mflg/internal/termesc"
)

// This file contains tests for behaviour that was at some point found to crash mflg.
// Most of these were found by fuzzing.

func TestUndoPastSelectionBound(t *testing.T) {
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

func TestMouseEventBelowBottom(t *testing.T) {
	app := application{}
	app.mainWindow = newTestWindow(t, 80, 25, "")
	app.resize(25, 80)
	app.handleMouseEvent(termesc.MouseEvent{Button: termesc.ScrollUpButton, Shift: true, Alt: true, Control: true, Move: false, X: 18, Y: 29})
}
