package main

import (
	"io"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/termesc"

	"golang.org/x/crypto/ssh/terminal"
)

type application struct {
	searchRE                 *regexp.Regexp // The regexp used in the last navigation command, if any
	navStack                 []location
	filename                 string
	mainWindow, promptWindow *window
	cursorVisible            bool
	width, height            int
	promptHandler            func(string) // What to do with the prompt input when the user hits Enter

	saveDelay        time.Duration
	saveTimer        *time.Timer
	saveTimerPending bool

	titleNeedsRedraw bool
}

type location struct {
	filename string
	line     int
}

func (app *application) navigateTo(where string) error {
	// If this isn't the very first navigation command, save the current location and add it to the
	// navigation stack once the command completes successfully.
	oldLocation := location{filename: app.filename, line: -1}
	if app.filename != "" {
		oldLocation.line = app.mainWindow.windowCoordsToTextCoords(app.mainWindow.cursorPos).Y
	}
	line := 1
	regex := (*regexp.Regexp)(nil)
	filename := where
	err := error(nil)
	if i := strings.IndexByte(where, ':'); i != -1 {
		filename = where[:i]
		if rest := where[i+1:]; allASCIIDigits(rest) {
			line, err = strconv.Atoi(rest)
		} else {
			regex, err = regexp.Compile(rest)
		}
		if err != nil {
			return err
		}
	}
	if filename != "" {
		filename = expandPath(filename)
		// Interpret relative paths relative to the directory containing the current file, if any.
		// When starting up, interpret them relative to the working directory.
		if !filepath.IsAbs(filename) {
			if app.filename != "" {
				filename = filepath.Join(filepath.Dir(app.filename), filename)
			} else {
				if filename, err = filepath.Abs(filename); err != nil {
					return err
				}
			}
		}
	}
	if err := app.gotoFile(filename); err != nil {
		return err
	}
	loc := location{filename: app.filename, line: 0}
	switch {
	case regex != nil:
		app.mainWindow.searchRegexp(regex, 0)
		loc.line = app.mainWindow.windowCoordsToTextCoords(app.mainWindow.cursorPos).Y
	case line > 0:
		app.mainWindow.gotoLine(line - 1)
		loc.line = line - 1
	}
	if oldLocation.line >= 0 {
		app.navStack = append(app.navStack, oldLocation)
	}
	app.searchRE = regex
	return nil
}

// expandPath expands references to environment variables in path, of the form $VAR or ${VAR}.
// It also expands ~/ at the start of a path to the user's home directory.
func expandPath(path string) string {
	path = os.ExpandEnv(path)
	if p := strings.TrimPrefix(path, "~"+string(filepath.Separator)); len(p) != len(path) {
		// In the unlikely event that the lookup fails, leave the tilde unexpanded; it will be easier
		// to detect the problem that way.
		if u, err := currentUser(); err == nil {
			path = filepath.Join(u.HomeDir, p)
		}
	}
	return path
}

// This is a variable so that it can be mocked for tests.
var currentUser = user.Current

// gotoFile loads the file at filename into the editor, if it isn't the currently open file already.
func (app *application) gotoFile(filename string) error {
	if filename != "" && filename != app.filename {
		buf := buffer.New()
		if f, err := os.Open(filename); err == nil {
			_, err = buf.ReadFrom(f)
			f.Close()
			if err != nil {
				return err
			}
			// Allow the user to edit a file that doesn't exist yet
		} else if !os.IsNotExist(err) {
			return err
		}
		app.saveNow()
		app.mainWindow = newWindow(app.width, app.height, buf)
		app.mainWindow.onChange = app.resetSaveTimer
		app.filename = filename
		app.titleNeedsRedraw = true
	}
	return nil
}

func (app *application) gotoNextMatch() {
	if app.searchRE != nil {
		y := app.mainWindow.windowCoordsToTextCoords(app.mainWindow.cursorPos).Y
		app.navStack = append(app.navStack, location{filename: app.filename, line: y})
		app.mainWindow.searchRegexp(app.searchRE, y+1)
	}
}

func (app *application) back() error {
	if len(app.navStack) == 0 {
		return nil
	}
	s := app.navStack
	loc := s[len(s)-1]
	if err := app.gotoFile(loc.filename); err != nil {
		return err
	}
	app.mainWindow.gotoLine(loc.line)
	app.navStack = s[:len(s)-1]
	return nil
}

func (app *application) currentFile() string { return app.filename }

func (app *application) resetSaveTimer() {
	if app.saveTimer == nil {
		app.saveTimer = time.NewTimer(app.saveDelay)
		app.saveTimerPending = true
		return
	}
	if !app.saveTimer.Stop() && app.saveTimerPending {
		<-app.saveTimer.C
	}
	app.saveTimer.Reset(app.saveDelay)
	app.saveTimerPending = true
}

func (app *application) saveNow() {
	if app.saveTimerPending {
		if !app.saveTimer.Stop() {
			<-app.saveTimer.C
		}
		saveBuffer(app.filename, app.mainWindow.buf)
		app.saveTimerPending = false
	}
}

func (app *application) saveTimerChan() <-chan time.Time {
	if app.saveTimer == nil {
		return nil
	}
	return app.saveTimer.C
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
				if app.saveTimerPending {
					saveBuffer(app.filename, app.mainWindow.buf)
				}
				return nil
			case "\x7f", "\b":
				aw.backspace()
			case "\x0c":
				app.openPrompt("Go to:", func(response string) {
					if err := app.navigateTo(response); err != nil {
						must(printAtBottom(err.Error()))
					}
				})
			case "\x07":
				app.gotoNextMatch()
			case "\x02":
				if err := app.back(); err != nil {
					must(printAtBottom(err.Error()))
				}
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
		case <-app.saveTimerChan():
			app.saveTimerPending = false
			if err := saveBuffer(app.filename, app.mainWindow.buf); err != nil {
				printAtBottom(err.Error())
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
	if app.titleNeedsRedraw {
		if _, err := console.Write([]byte(termesc.SetTitle(app.filename))); err != nil {
			return err
		}
		app.titleNeedsRedraw = false
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
