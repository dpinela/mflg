package main

import (
	"io"

	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/termesc"
)

type application struct {
	filename                 string
	mainWindow, promptWindow *window
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

func (app *application) openPrompt(prompt string, whenDone func(string)) {
	if app.promptWindow == nil {
		app.promptWindow = newWindow(app.width, 1, buffer.New())
		app.promptWindow.customGutterText = prompt
		app.promptHandler = whenDone
	}
}

func (app *application) cancelPrompt() {
	app.mainWindow.needsRedraw = true
	app.promptWindow = nil
	app.promptHandler = nil
}

func (app *application) finishPrompt() {
	app.promptHandler(app.promptWindow.buf.Line(1))
	app.cancelPrompt()
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
	p := app.cursorPos()
	if app.promptWindow != nil {
		if err := app.promptWindow.redrawAtYOffset(console, app.promptYOffset()); err != nil {
			return err
		}
	}
	if err := app.mainWindow.redraw(console); err != nil {
		return err
	}
	_, err := console.Write([]byte(termesc.SetCursorPos(p.y+1, p.x+app.activeWindow().gutterWidth()+1)))
	return err
}

func (app *application) cursorPos() point {
	if app.promptWindow != nil {
		return point{app.promptWindow.cursorPos.x, app.promptWindow.cursorPos.y + app.promptYOffset()}
	}
	return app.mainWindow.cursorPos
}

func (app *application) handleMouseEvent(ev termesc.MouseEvent) {
	if py := app.promptYOffset(); ev.Y >= py {
		ev.Y -= py
		app.promptWindow.handleMouseEvent(ev)
		return
	}
	app.mainWindow.handleMouseEvent(ev)
}
