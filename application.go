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

	"github.com/dpinela/charseg"
	"github.com/fsnotify/fsnotify"
	"github.com/mattn/go-runewidth"

	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/config"
	"github.com/dpinela/mflg/internal/highlight"
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
	note                     string
	noteClearTimer           timer

	saveDelay      time.Duration
	saveTimer      timer
	taskQueue      chan func() // Used by asynchronous tasks to run code on the main event loop
	fsWatcher      *fsnotify.Watcher
	watchTarget    string
	removeTimer    timer
	createDirTimer timer

	// These fields are used when receiving a bracketed paste
	pasteBuffer      []byte
	inBracketedPaste bool

	titleNeedsRedraw bool

	config *config.Config
}

// A wrapper for time.Timer that can be safely reset.
// The zero value is an inactive, usable timer.
// After receiving on the timer t's channel, the receiver must set t.pending to false.
type timer struct {
	timer   *time.Timer
	pending bool
}

func (t *timer) reset(dt time.Duration) {
	if t.timer == nil {
		t.timer = time.NewTimer(dt)
		t.pending = true
		return
	}
	t.stop()
	t.timer.Reset(dt)
	t.pending = true
}

func (t *timer) stop() {
	if t.timer != nil && !t.timer.Stop() && t.pending {
		<-t.timer.C
	}
	t.pending = false
}

func (t *timer) channel() <-chan time.Time {
	if t.timer == nil {
		return nil
	}
	return t.timer.C
}

type location struct {
	filename string
	pos      point
}

func (app *application) navigateTo(where string) error {
	// If this isn't the very first navigation command, save the current location and add it to the
	// navigation stack once the command completes successfully.
	oldLocation := location{filename: app.filename, pos: point{-1, -1}}
	if app.filename != "" {
		oldLocation.pos = app.mainWindow.windowCoordsToTextCoords(app.mainWindow.cursorPos)
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
	switch {
	case regex != nil:
		app.mainWindow.searchRegexp(regex, 0)
	case line > 0:
		app.mainWindow.gotoLine(line - 1)
	}
	if oldLocation.pos.Y >= 0 {
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
		// If we fail to set up the watch, we still want to open the file.
		if app.fsWatcher == nil {
			if w, err := fsnotify.NewWatcher(); err == nil {
				app.fsWatcher = w
			}
		}
		app.removeTimer.stop()
		app.createDirTimer.stop()
		if app.fsWatcher != nil {
			app.setupMainWatch(filename)
		}
		app.finishFormatNow()
		app.saveNow()
		app.mainWindow = newWindow(app.width, app.height, buf, app.config.TabWidth)
		app.mainWindow.onChange = app.resetSaveTimer
		if ext := filepath.Ext(filename); ext != "" {
			app.mainWindow.langConfig = app.config.ConfigForExt(ext[1:])
			app.mainWindow.highlighter = highlight.Language(ext[1:], app.mainWindow, &highlight.Palette{
				Comment: highlight.Style{Foreground: highlight.Color{Color: *app.config.TextStyle.Comment.Foreground, Alpha: true}},
				String:  highlight.Style{Foreground: highlight.Color{Color: *app.config.TextStyle.String.Foreground, Alpha: true}},
			})
		} else {
			app.mainWindow.highlighter = highlight.Language(ext, app.mainWindow, &highlight.Palette{})
		}
		app.mainWindow.app = app
		app.filename = filename
		app.titleNeedsRedraw = true
	}
	return nil
}

func (app *application) setupMainWatch(filename string) error {
	if app.watchTarget != "" {
		app.fsWatcher.Remove(app.watchTarget)
	}
	parent := filepath.Dir(filename)
	var err error
	for {
		if err = app.fsWatcher.Add(parent); err == nil {
			break
		}
		next := filepath.Dir(parent)
		if next == parent {
			break
		}
		parent = next
	}
	if err == nil {
		app.watchTarget = parent
	} else {
		app.watchTarget = ""
	}
	return err
}

func (app *application) reloadFile() error {
	buf := buffer.New()
	if f, err := os.Open(app.filename); err == nil {
		_, err = buf.ReadFrom(f)
		f.Close()
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	app.mainWindow.buf = buf
	app.mainWindow.wrappedBuf.Reset(buf)
	app.mainWindow.highlighter.Invalidate(0)
	app.mainWindow.roundCursorPos()
	app.mainWindow.needsRedraw = true
	if app.promptWindow != nil {
		app.promptWindow.needsRedraw = true
	}
	return nil
}

func (app *application) fsWatcherChannels() (<-chan fsnotify.Event, <-chan error) {
	if app.fsWatcher != nil {
		return app.fsWatcher.Events, app.fsWatcher.Errors
	}
	return nil, nil
}

func (app *application) gotoNextMatch() {
	if app.searchRE != nil {
		tp := app.mainWindow.windowCoordsToTextCoords(app.mainWindow.cursorPos)
		app.navStack = append(app.navStack, location{filename: app.filename, pos: tp})
		app.mainWindow.searchRegexp(app.searchRE, tp.Y+1)
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
	app.mainWindow.gotoTextPos(loc.pos)
	app.navStack = s[:len(s)-1]
	return nil
}

func (app *application) currentFile() string { return app.filename }

func (app *application) resetSaveTimer() { app.saveTimer.reset(app.saveDelay) }

func (app *application) saveNow() {
	if app.saveTimer.pending {
		if !app.saveTimer.timer.Stop() {
			<-app.saveTimer.timer.C
		}
		saveBuffer(app.filename, app.mainWindow.buf)
		app.saveTimer.pending = false
	}
}

func (app *application) finishFormatNow() {
	for app.mainWindow != nil && app.mainWindow.formatPending {
		(<-app.taskQueue)()
	}
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
		fsEvents, fsWatchErrors := app.fsWatcherChannels()
		select {
		case c, ok := <-inputCh:
			if !ok {
				return nil
			}
			if app.inBracketedPaste {
				if c == termesc.PastedTextEnd {
					aw.insertText(app.pasteBuffer)
					app.inBracketedPaste = false
				} else {
					if c == "\r" {
						c = "\n"
					}
					app.pasteBuffer = append(app.pasteBuffer, c...)
				}
				continue
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
			case termesc.PastedTextBegin:
				app.inBracketedPaste = true
				app.pasteBuffer = app.pasteBuffer[:0]
			case "\x11":
				app.finishFormatNow()
				if app.saveTimer.pending {
					saveBuffer(app.filename, app.mainWindow.buf)
				}
				return nil
			case "\x7f", "\b":
				aw.backspace()
			case "\x0c":
				app.openPrompt("Go to:", func(response string) {
					if err := app.navigateTo(response); err != nil {
						app.setNotification(err.Error())
					}
				})
			case "\x07":
				app.gotoNextMatch()
			case "\x02":
				if err := app.back(); err != nil {
					app.setNotification(err.Error())
				}
			case "\x06":
				aw.formatBuffer()
			case "\x12":
				app.openPrompt("Replace:", func(searchRE string) {
					re, err := regexp.Compile(searchRE)
					if err != nil {
						app.setNotification(err.Error())
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
		case <-app.saveTimer.channel():
			app.saveTimer.pending = false
			if err := saveBuffer(app.filename, app.mainWindow.buf); err != nil {
				app.setNotification(err.Error())
			}
		case <-app.noteClearTimer.channel():
			app.noteClearTimer.pending = false
			app.note = ""
			app.mainWindow.needsRedraw = true
		case <-app.removeTimer.channel():
			app.removeTimer.pending = false
			app.reloadFile()
		case <-app.createDirTimer.channel():
			app.createDirTimer.pending = false
			app.reloadFile()
			app.setupMainWatch(app.filename)
		case ev := <-fsEvents:
			p, err := filepath.Abs(ev.Name)
			if err != nil {
				continue
			}
			switch {
			case p == app.filename:
				if ev.Op&(fsnotify.Write|fsnotify.Create) != 0 {
					app.reloadFile()
				}
				// When a file is renamed on top of the one we're looking at,
				// fsnotify may report that as a Remove immediately followed
				// by a Create. To avoid the window flickering briefly when
				// that happens, we wait a bit to confirm the file really was deleted.
				if ev.Op&fsnotify.Remove != 0 {
					app.removeTimer.reset(time.Second / 10)
				}
				if ev.Op&fsnotify.Create != 0 {
					app.removeTimer.stop()
				}
			case strings.HasPrefix(app.filename, addTrailingSeparator(p)):
				switch ev.Op & (fsnotify.Create | fsnotify.Remove) {
				case fsnotify.Create:
					app.createDirTimer.reset(15 * time.Millisecond)
				case fsnotify.Remove, fsnotify.Create | fsnotify.Remove:
					app.reloadFile()
					app.setupMainWatch(app.filename)
				}
			}
		case err := <-fsWatchErrors:
			app.setNotification(err.Error())
		case f := <-app.taskQueue:
			f()
		}
	}
}

func addTrailingSeparator(path string) string {
	if strings.HasSuffix(path, string(filepath.Separator)) {
		return path
	}
	return path + string(filepath.Separator)
}

// do schedules f to run on the main event loop.
// It is safe to call it concurrently only from outside the goroutine running app.run.
// Calling it from that goroutine may deadlock.
func (app *application) do(f func()) {
	app.taskQueue <- f
}

func (app *application) resize(height, width int) {
	app.width = width
	app.height = height
	app.mainWindow.resize(app.height, app.width)
	if app.promptWindow != nil {
		app.promptWindow.resize(1, app.width)
	}
}

// openPrompt opens a prompt window at the bottom of the viewport.
// When the user hits Enter, whenDone is called with the entered text.
func (app *application) openPrompt(prompt string, whenDone func(string)) {
	app.promptWindow = newWindow(app.width, 1, buffer.New(), 4)
	app.promptWindow.setGutterText(prompt)
	app.promptWindow.highlighter = highlight.Language("", app.promptWindow, &highlight.Palette{})
	app.promptHandler = whenDone
	app.note = ""
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

// setNotification displays a string at the bottom line of the viewport until the next
// call to this or openPrompt.
func (app *application) setNotification(note string) {
	app.note = note
	app.noteClearTimer.reset(5 * time.Second)
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
	var err error
	if err := app.mainWindow.redraw(console); err != nil {
		return err
	}
	switch {
	case app.promptWindow != nil:
		err = app.promptWindow.redrawAtYOffset(console, app.promptYOffset())
	case app.note != "":
		_, err = console.Write([]byte(termesc.SetCursorPos(app.height, 0) + styleResetToBold + ellipsify(app.note, app.width)))
	}
	if err != nil {
		return err
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
		_, err = console.Write([]byte(termesc.SetCursorPos(p.Y+1, p.X+app.activeWindow().gutterWidth()+1)))
		return err
	} else if app.cursorVisible {
		_, err = console.Write([]byte(termesc.HideCursor))
		return err
	}
	return nil
}

var ellipses = [...]string{"", ".", "..", "..."}

// ellipsify truncates text to fit within width columns, adding an ellipsis at the end if it
// must be truncated.
func ellipsify(text string, width int) string {
	if i := strings.IndexByte(text, '\n'); i != -1 {
		text = text[:i]
	}
	text = strings.Replace(text, "\t", " ", -1)
	if n := runewidth.StringWidth(text); n <= width {
		return text
	}
	n := 0
	prefix := ""
	prefixSet := false
	for i := 0; i < len(text); {
		c := charseg.FirstGraphemeCluster(text[i:])
		w := runewidth.StringWidth(c)
		// Record the part of the string that fits without counting the ellipsis.
		if n+w > width-3 && !prefixSet {
			prefix = text[:i]
			prefixSet = true
		}
		if n+w > width {
			return prefix + ellipses[min(3, width)]
		}
		n += w
		i += len(c)
	}
	return text
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
