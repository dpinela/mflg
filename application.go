package main

import (
	"io"

	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/termesc"
)

type application struct {
	filename                 string
	mainWindow, promptWindow *window
	cursorVisible            bool
	width, height            int
	promptHandler            func(string) // What to do with the prompt input when the user hits Enter
}

func (app *application) resize(height, width int) {
	app.width = width
	app.height = height
	app.mainWindow.resize(app.height, app.width)
	if app.promptWindow != nil {
		app.promptWindow.resize(1, app.width)
	}
}

func (app *application) openFile(filename string) error {
	return nil
}

// openPrompt opens a prompt window at the bottom of the viewport.
// When the user hits Enter, whenDone is called with the entered text.
func (app *application) openPrompt(prompt string, whenDone func(string)) {
	app.promptWindow = newWindow(app.width, 1, buffer.New())
	app.promptWindow.setGutterText(prompt)
	app.promptHandler = whenDone
}

func (app *application) cancelPrompt() {
	app.mainWindow.needsRedraw = true
	app.promptWindow = nil
	app.promptHandler = nil
}

func (app *application) finishPrompt() {
	// Do things in this order so that the prompt handler can safely call openPrompt.
	response := app.promptWindow.buf.Line(0)
	handler := app.promptHandler
	app.cancelPrompt()
	handler(response)
}

func (app *application) activeWindow() *window {
	if app.promptWindow != nil {
		return app.promptWindow
	}
	return app.mainWindow
}

func (app *application) promptYOffset() int {
	if app.promptWindow != nil {
		return app.height - 1
	}
	return app.height
}

func (app *application) redraw(console io.Writer) error {
	if err := app.mainWindow.redraw(console); err != nil {
		return err
	}
	if app.promptWindow != nil {
		if err := app.promptWindow.redrawAtYOffset(console, app.promptYOffset()); err != nil {
			return err
		}
	}
	nowVisible := app.activeWindow().cursorInViewport()
	defer func() { app.cursorVisible = nowVisible }()
	if nowVisible {
		if !app.cursorVisible {
			if _, err := console.Write([]byte(termesc.ShowCursor)); err != nil {
				return err
			}
		}
		p := app.cursorPos()
		_, err := console.Write([]byte(termesc.SetCursorPos(p.y+1, p.x+app.activeWindow().gutterWidth()+1)))
		return err
	} else if app.cursorVisible {
		_, err := console.Write([]byte(termesc.HideCursor))
		return err
	}
	return nil
}

func (app *application) cursorPos() point {
	if app.promptWindow != nil {
		p := app.promptWindow.viewportCursorPos()
		return point{p.x, p.y + app.promptYOffset()}
	}
	return app.mainWindow.viewportCursorPos()
}

func (app *application) handleMouseEvent(ev termesc.MouseEvent) {
	if py := app.promptYOffset(); ev.Y >= py {
		ev.Y -= py
		app.promptWindow.handleMouseEvent(ev)
		return
	}
	if app.promptWindow != nil && !ev.Move {
		app.cancelPrompt()
	}
	app.mainWindow.handleMouseEvent(ev)
}
