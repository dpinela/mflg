package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strconv"

	"github.com/dpinela/mflg/internal/atomicwrite"
	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/termesc"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sys/unix"
)

func saveBuffer(fname string, buf *buffer.Buffer) error {
	return atomicwrite.Write(fname, func(w io.Writer) error { _, err := buf.WriteTo(w); return err })
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func printAtBottom(text string) error {
	_, err := fmt.Printf("%s%s%s", termesc.SetCursorPos(2000, 1), termesc.ClearLine, text)
	return err
}

func allASCIIDigits(s string) bool {
	for i := range s {
		if !(s[i] >= '0' && s[i] <= '9') {
			return false
		}
	}
	return true
}

// A mflg instance is made of three components:
// - a main window, which handles text editing for the open file
// - a prompt window, which provides the same functionality for the text entered in response
//   to various command prompts.
// - an application object, which coordinates rendering of these two windows, and distributes input
//   between them as appropriate.

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage:", os.Args[0], "<file>")
		os.Exit(2)
	}
	buf := buffer.New()
	fname := os.Args[1]
	if f, err := os.Open(fname); err == nil {
		_, err = buf.ReadFrom(f)
		f.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error reading %s: %v", fname, err)
			os.Exit(2)
		}
	} else if !os.IsNotExist(err) {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	w, h, err := terminal.GetSize(0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error finding terminal size:", err)
		os.Exit(2)
	}
	oldMode, err := terminal.MakeRaw(0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error entering raw mode:", err)
		os.Exit(2)
	}
	defer terminal.Restore(0, oldMode)
	app := application{mainWindow: newWindow(w, h, buf)}
	app.resize(h, w)
	os.Stdout.WriteString(termesc.EnableMouseReporting + termesc.EnterAlternateScreen)
	defer os.Stdout.WriteString(termesc.ExitAlternateScreen + termesc.ShowCursor + termesc.DisableMouseReporting)
	resizeCh := make(chan os.Signal, 32)
	inputCh := make(chan string, 32)
	go func() {
		con := termesc.NewConsoleReader(os.Stdin)
		for {
			if s, err := con.ReadToken(); err != nil {
				close(inputCh)
				return
			} else {
				inputCh <- s
			}
		}
	}()
	signal.Notify(resizeCh, unix.SIGWINCH)
	for {
		must(app.redraw(os.Stdout))
		aw := app.activeWindow()
		select {
		case c, ok := <-inputCh:
			if !ok {
				panic("console input closed")
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
					return
				}
				must(printAtBottom("Discard changes [y/N]? "))
				if c = <-inputCh; len(c) == 1 && (c[0] == 'y' || c[0] == 'Y') {
					return
				}
			case "\x13":
				if !app.mainWindow.dirty {
					continue
				}
				if err := saveBuffer(fname, buf); err != nil {
					must(printAtBottom(err.Error()))
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
				aw.resetSelectionState()
			case "\x03":
				aw.copySelection()
			case "\x16":
				aw.paste()
			case "\x1b":
				if app.promptWindow != nil {
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
				}
			}
		case <-resizeCh:
			// This can only fail if our terminal turns into a non-terminal
			// during execution, which is highly unlikely.
			if w, h, err := terminal.GetSize(0); err != nil {
				panic(err)
			} else {
				app.resize(h, w)
			}
		}
	}
}
