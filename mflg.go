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

func mustWrite(w io.Writer, b []byte) {
	if _, err := w.Write(b); err != nil {
		panic(err)
	}
}

// Predefined []byte strings to avoid allocations.
var (
	crlf       = []byte("\r\n")
	tab        = []byte("\t")
	fourSpaces = []byte("    ")
)

func saveBuffer(fname string, buf *buffer.Buffer) error {
	return atomicwrite.Write(fname, func(w io.Writer) error { _, err := buf.WriteTo(w); return err })
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func rawGetLine(in <-chan string, out io.Writer) (string, error) {
	var line []byte
	for {
		c := <-in
		if len(c) == 0 {
			return string(line), nil
		}
		if len(c) == 1 && c[0] == '\r' {
			return string(line), nil
		}
		if _, err := fmt.Fprint(out, c); err != nil {
			return string(line), err
		}
		line = append(line, c...)
	}
}

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
	win := newWindow(os.Stdout, w, h, buf)
	os.Stdout.WriteString(termesc.EnableMouseReporting + termesc.EnterAlternateScreen)
	defer os.Stdout.WriteString(termesc.ExitAlternateScreen + termesc.DisableMouseReporting)
	resizeCh := make(chan os.Signal, 32)
	inputCh := make(chan string, 32)
	go func() {
		con := termesc.NewConsoleReader(os.Stdin)
		for {
			if s, err := con.ReadToken(); err != nil {
				close(inputCh)
			} else {
				inputCh <- s
			}
		}
	}()
	signal.Notify(resizeCh, unix.SIGWINCH)
	var c string
	for {
		if err := win.redraw(true); err != nil {
			panic(err)
		}
		fmt.Fprint(win.w, termesc.SetCursorPos(win.cursorPos.y+1, win.cursorPos.x+win.gutterWidth()+1))
		select {
		case k, ok := <-inputCh:
			if !ok {
				panic("console input closed")
			}
			c = k
		case <-resizeCh:
			// This can only fail if our terminal turns into a non-terminal
			// during execution, which is highly unlikely.
			if w, h, err := terminal.GetSize(0); err != nil {
				panic(err)
			} else {
				win.resize(h, w)
			}
			continue
		}
		switch c {
		case termesc.UpKey:
			win.repeatMove(win.moveCursorUp)
		case termesc.DownKey:
			win.repeatMove(win.moveCursorDown)
		case termesc.LeftKey:
			win.moveCursorLeft()
		case termesc.RightKey:
			win.moveCursorRight()
		case "\x11":
			if !win.dirty {
				return
			}
			must(win.printAtBottom("Discard changes [y/N]? "))
			if c = <-inputCh; len(c) == 1 && (c[0] == 'y' || c[0] == 'Y') {
				return
			}
		case "\x13":
			if !win.dirty {
				continue
			}
			if err := saveBuffer(fname, buf); err != nil {
				must(win.printAtBottom(err.Error()))
			} else {
				win.dirty = false
			}
		case "\x7f", "\b":
			win.backspace()
		case "\x0c":
			must(win.printAtBottom("Go to line: "))
			lineStr, err := rawGetLine(inputCh, win.w)
			must(err)
			y, err := strconv.ParseInt(lineStr, 10, 32)
			if err == nil {
				win.gotoLine(int(y - 1))
			}
		case "\x06":
			must(win.printAtBottom("Search: "))
			reText, err := rawGetLine(inputCh, win.w)
			must(err)
			re, err := regexp.Compile(reText)
			if err != nil {
				must(win.printAtBottom(err.Error()))
			} else {
				win.searchRegexp(re)
			}
		case "\x01":
			if !win.inMouseSelection() {
				win.markSelectionBound()
			}
		case "\x18":
			win.resetSelectionState()
		case "\x03":
			win.copySelection()
		case "\x16":
			win.paste()
		default:
			if ev, err := termesc.ParseMouseEvent(c); err == nil {
				win.handleMouseEvent(ev)
			} else if len(c) > 0 && (c[0] >= ' ' || c[0] == '\r' || c[0] == '\t') {
				win.typeText(c)
			}
		}
	}
}
