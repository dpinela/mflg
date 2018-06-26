package main

import (
	"testing"

	"github.com/dpinela/mflg/internal/highlight"
	"github.com/dpinela/mflg/internal/termesc"
	"github.com/dpinela/mflg/internal/config"
)

// This file contains tests for behaviour that was at some point found to crash mflg.
// Most of these were found by fuzzing.

func TestUndoPastSelectionBound(t *testing.T) {
	w := newTestWindow(t, 80, 25, "")
	w.highlighter = highlight.Language("", w, &highlight.Palette{})
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
	app := application{config: &config.Config{}}
	app.mainWindow = newTestWindow(t, 80, 25, "")
	app.mainWindow.app = &app
	app.resize(25, 80)
	app.handleMouseEvent(termesc.MouseEvent{Button: termesc.ScrollUpButton, Shift: true, Alt: true, Control: true, Move: false, X: 18, Y: 29})
}
