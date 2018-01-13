package main

import (
	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/termesc"
	"testing"
)

const (
	stdWidth  = 40
	stdHeight = 20
)

func TestMouseEventsOutsidePrompt(t *testing.T) {
	app := &application{mainWindow: newWindow(stdWidth, stdHeight, buffer.New()), promptWindow: newWindow(stdWidth, 1, buffer.New()), width: stdWidth, height: stdHeight}
	app.handleMouseEvent(termesc.MouseEvent{X: 5, Y: 5, Move: true, Button: termesc.NoButton})
	if app.promptWindow == nil {
		t.Error("after mouse move outside prompt, prompt window was closed, shouldn't have been")
	}
	app.handleMouseEvent(termesc.MouseEvent{X: 6, Y: 6, Button: termesc.ReleaseButton})
	if app.promptWindow != nil {
		t.Error("after mouse click ouside prompt, prompt window wasn't closed, should have been")
	}
}
