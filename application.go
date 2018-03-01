package main

import (
	"io"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/termesc"

	"golang.org/x/crypto/ssh/terminal"
)

type application struct {
	filename                 string
	mainWindow, promptWindow *window
	cursorVisible            bool
	width, height            int
	promptHandler            func(string) // What to do with the prompt input when the user hits Enter

	saveDelay time.Duration
}

func (app *application) navigateTo(location string) error {
	buf := buffer.New()
	if f, err := os.Open(location); err == nil {
		_, err = buf.ReadFrom(f)
		f.Close()
		if err != nil {
			return err
		}
		// Allow the user to edit a file that doesn't exist yet
	} else if !os.IsNotExist(err) {
		return err
	}
	app.mainWindow = newWindow(0, 0, buf)
	app.filename = location
	return nil
}

func (app *application) run(in io.Reader, resizeSignal <-chan os.Signal, out io.Writer) error {
	inputCh := make(chan string, 32)
	go func() {
		con := termesc.NewConsoleReader(in)
		for {
			if s, err := con.ReadToken(); err != nil {
				close(inputCh)
				return
			} else {
				inputCh <- s
			}
		}
	}()
	for {
		if err := app.redraw(out); err != nil {
			return err
		}
		aw := app.activeWindow()
		select {
		case c, ok := <-inputCh:
			if !ok {
				return nil
			}
			switch c {
			case termesc.UpKey:
				aw.repeatMove(aw.moveCursorUp)
			case termesc.DownKey:
				aw.repeatMove(aw.moveCursorDown)
			case termesc.LeftKey:
				aw.moveCursorLeft()
			case termesc.RightKey:
				aw.moveCursorRight()
			case "\x11":
				if !app.mainWindow.dirty {
					return nil
				}
				if err := printAtBottom("Discard changes [y/N]? "); err != nil {
					return err
				}
				if c = <-inputCh; c == "y" || c == "Y" {
					return nil
				}
			case "\x13":
				if !app.mainWindow.dirty {
					continue
				}
				if err := saveBuffer(app.filename, app.mainWindow.buf); err != nil {
					if err := printAtBottom(err.Error()); err != nil {
						return err
					}
				} else {
					app.mainWindow.dirty = false
				}
			case "\x7f", "\b":
				aw.backspace()
			case "\x0c":
				app.openPrompt("Go to:", func(response string) {
					if allASCIIDigits(response) {
						lineNum, err := strconv.ParseInt(response, 10, 64)
						if err != nil {
							must(printAtBottom(err.Error()))
							return
						}
						if lineNum > 0 {
							app.mainWindow.gotoLine(int(lineNum - 1))
						}
					} else {
						re, err := regexp.Compile(response)
						if err != nil {
							must(printAtBottom(err.Error()))
							return
						}
						app.mainWindow.searchRegexp(re)
					}
				})
			case "\x12":
				app.openPrompt("Replace:", func(searchRE string) {
					re, err := regexp.Compile(searchRE)
					if err != nil {
						must(printAtBottom(err.Error()))
						return
					}
					app.openPrompt("With:", func(replacement string) {
						app.mainWindow.replaceRegexp(re, replacement)
					})
				})
			case "\x01":
				if !aw.inMouseSelection() {
					aw.markSelectionBound()
				}
			case "\x18":
				aw.cutSelection()
			case "\x03":
				aw.copySelection()
			case "\x16":
				aw.paste()
			case "\x1a":
				aw.undo()
			case "\x15":
				if len(aw.undoStack) > 0 && app.promptWindow == nil {
					app.openPrompt("Discard changes [y/Esc]?", func(resp string) {
						if len(resp) != 0 && (resp[0] == 'Y' || resp[0] == 'y') {
							aw.undoAll()
						}
					})
				} else {
					aw.undoAll()
				}
			case "\x1b":
				switch {
				case aw.selection.Set || aw.selectionAnchor.Set || aw.mouseSelectionAnchor.Set:
					aw.resetSelectionState()
				case app.promptWindow != nil:
					app.cancelPrompt()
				}
			default:
				if ev, err := termesc.ParseMouseEvent(c); err == nil {
					app.handleMouseEvent(ev)
				} else if c >= " " || c == "\r" || c == "\t" {
					if app.promptWindow != nil && c == "\r" {
						app.finishPrompt()
					} else {
						aw.typeText(c)
					}
				} else if termesc.IsAltRightKey(c) {
					aw.moveCursorRightWord()
				} else if termesc.IsAltLeftKey(c) {
					aw.moveCursorLeftWord()
				}
			}
		case <-resizeSignal:
			// This can only fail if our terminal turns into a non-terminal
			// during execution, which is highly unlikely.
			if w, h, err := terminal.GetSize(0); err != nil {
				return err
			} else {
				app.resize(h, w)
			}
		}
	}
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
		_, err := console.Write([]byte(termesc.SetCursorPos(p.Y+1, p.X+app.activeWindow().gutterWidth()+1)))
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
		return point{p.X, p.Y + app.promptYOffset()}
	}
	return app.mainWindow.viewportCursorPos()
}

func (app *application) handleMouseEvent(ev termesc.MouseEvent) {
	if py := app.promptYOffset(); ev.Y >= py && app.promptWindow != nil {
		ev.Y -= py
		app.promptWindow.handleMouseEvent(ev)
		return
	}
	if app.promptWindow != nil && !ev.Move {
		app.cancelPrompt()
	}
	app.mainWindow.handleMouseEvent(ev)
}
