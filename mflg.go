package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
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

func saveBuffer(fname string, buf *buffer.Buffer) error {
	tf, err := ioutil.TempFile("", "mflg-tmp-")
	if err != nil {
		return err
	}
	if _, err := buf.WriteTo(tf); err != nil {
		return err
	}
	if err = tf.Close(); err != nil {
		return err
	}
	return os.Rename(tf.Name(), fname)
	/*if _, err := f.Seek(0, os.SEEK_SET); err != nil {
		return err
	}
	if _, err := buf.WriteTo(f); err != nil {
		return err
	}
	return nil*/
}

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func rawGetLine(in io.Reader, out io.Writer) (string, error) {
	var b [8]byte
	var line []byte
	for {
		n, err := in.Read(b[:])
		if err != nil {
			return string(line), err
		}
		if n == 1 && b[0] == '\r' {
			return string(line), nil
		}
		if _, err := out.Write(b[:n]); err != nil {
			return string(line), err
		}
		line = append(line, b[:n]...)
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
	win := window{w: os.Stdout, width: w, height: h, buf: buf, needsRedraw: true}
	defer terminal.Restore(int(os.Stdin.Fd()), oldMode)
	enterAlternateScreen()
	defer exitAlternateScreen()
	for {
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
			if !win.dirty {
				return
			}
			must(win.printAtBottom("[S]ave/[D]iscard changes/[C]ancel? "))
			if m, err := os.Stdin.Read(b[:]); err == nil {
				if m == 1 {
					switch b[0] {
					case 's', 'S':
						if err := saveBuffer(fname, buf); err != nil {
							must(win.printAtBottom(err.Error()))
						} else {
							return
						}
					case 'd', 'D':
						return
					}
				}
			}
		case n == 1 && b[0] == '\x7f':
			win.backspace()
		case n == 1 && b[0] == '\f':
			must(win.printAtBottom("Go to line: "))
			lineStr, err := rawGetLine(os.Stdin, win.w)
			must(err)
			y, err := strconv.ParseInt(lineStr, 10, 32)
			if err == nil {
				win.gotoLine(int(y - 1))
			}
		case n == 1 && b[0] == 6:
			must(win.printAtBottom("Search: "))
			reText, err := rawGetLine(os.Stdin, win.w)
			must(err)
			re, err := regexp.Compile(reText)
			if err != nil {
				must(win.printAtBottom(err.Error()))
			} else {
				win.searchRegexp(re)
			}
		case n > 0 && (b[0] >= ' ' || b[0] == '\r' || b[0] == '\t'):
			win.typeText(b[:n])
		}
	}
}
