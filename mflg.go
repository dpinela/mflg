package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"unicode/utf8"

	"github.com/dpinela/mflg/buffer"
	"golang.org/x/crypto/ssh/terminal"
)

func enterAlternateScreen() {
	os.Stdout.WriteString("\033[?1049h")
}

func exitAlternateScreen() {
	os.Stdout.WriteString("\033[?1049l")
}

func mustWrite(w io.Writer, b []byte) {
	if _, err := w.Write(b); err != nil {
		panic(err)
	}
}

func gotoTop(w io.Writer) {
	mustWrite(w, resetScreen)
}

func gotoPos(w io.Writer, row, col int) {
	if _, err := fmt.Fprintf(w, "\033[%d;%dH", row+1, col+1); err != nil {
		panic(err)
	}
}

// Returns buf truncated to the first n runes.
func truncateToWidth(buf []byte, n int) []byte {
	j := 0
	for i := 0; i < n && len(buf[j:]) > 0; i++ {
		_, n := utf8.DecodeRune(buf[j:])
		j += n
	}
	return buf[:j]
}

// Predefined []byte strings to avoid allocations.
var (
	resetScreen = []byte("\033[;H\033[;2J")

	crlf       = []byte("\r\n")
	tab        = []byte("\t")
	fourSpaces = []byte("    ")

	upKey    = []byte("\033[A")
	downKey  = []byte("\033[B")
	leftKey  = []byte("\033[D")
	rightKey = []byte("\033[C")
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage:", os.Args[0], "<file>")
		os.Exit(2)
	}
	fname := os.Args[1]
	f, err := os.OpenFile(fname, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	buf := buffer.New()
	if _, err = buf.ReadFrom(f); err != nil {
		fmt.Fprintf(os.Stderr, "error reading %s: %v", fname, err)
		os.Exit(2)
	}
	w, h, err := terminal.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error finding terminal size:", err)
		os.Exit(2)
	}
	oldMode, err := terminal.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error entering raw mode:", err)
		os.Exit(2)
	}
	win := window{w: os.Stdout, width: w, height: h, buf: buf}
	defer terminal.Restore(int(os.Stdin.Fd()), oldMode)
	enterAlternateScreen()
	defer exitAlternateScreen()
	for {
		gotoTop(os.Stdout)
		if err := win.renderBuffer(); err != nil {
			panic(err)
		}
		gotoPos(os.Stdout, win.cursorY, win.cursorX+4)
		var b [8]byte
		n, err := os.Stdin.Read(b[:])
		if err != nil {
			return
		}
		switch {
		case bytes.Equal(b[:n], upKey):
			win.moveCursorUp()
		case bytes.Equal(b[:n], downKey):
			win.moveCursorDown()
		case bytes.Equal(b[:n], leftKey):
			win.moveCursorLeft()
		case bytes.Equal(b[:n], rightKey):
			win.moveCursorRight()
		case n == 1 && b[0] == '\x11':
			return
		case n == 1 && b[0] == '\x7f':
			win.backspace()
		case n > 0 && b[0] != '\033':
			win.typeText(b[:n])
			//win.typeText([]byte(strconv.Quote(string(b[:n]))))
		}
	}
}
