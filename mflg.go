package main

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/signal"
	"regexp"
	"strconv"
	"unicode/utf8"

	"github.com/dpinela/mflg/buffer"
	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sys/unix"
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

func rawGetLine(in <-chan []byte, out io.Writer) (string, error) {
	var line []byte
	for {
		c := <-in
		if len(c) == 0 {
			return string(line), nil
		}
		if len(c) == 1 && c[0] == '\r' {
			return string(line), nil
		}
		if _, err := out.Write(c); err != nil {
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
	resizeCh := make(chan os.Signal, 32)
	inputCh := make(chan []byte, 32)
	go func() {
		var b [8]byte
		for {
			if n, err := os.Stdin.Read(b[:]); err != nil {
				inputCh <- nil
			} else {
				inputCh <- b[:n]
			}
		}
	}()
	signal.Notify(resizeCh, unix.SIGWINCH)
	var c []byte
	for {
		if err := win.renderBuffer(); err != nil {
			panic(err)
		}
		gotoPos(os.Stdout, win.cursorY, win.cursorX+win.gutterWidth())
		select {
			case c = <-inputCh:
			case <-resizeCh:
				// This can only fail if our terminal turns into a non-terminal
				// during execution, or if we change os.Stdin for some reason
				if w, h, err := terminal.GetSize(int(os.Stdin.Fd())); err != nil {
					panic(err)
				} else {
					win.resize(h, w)
				}
		}
		switch {
		case bytes.Equal(c, upKey):
			win.moveCursorUp()
		case bytes.Equal(c, downKey):
			win.moveCursorDown()
		case bytes.Equal(c, leftKey):
			win.moveCursorLeft()
		case bytes.Equal(c, rightKey):
			win.moveCursorRight()
		case len(c) == 1 && c[0] == '\x11':
			if !win.dirty {
				return
			}
			must(win.printAtBottom("[S]ave/[D]iscard changes/[C]ancel? "))
			c = <-inputCh
			if len(c) == 1 {
				switch c[0] {
					case 's', 'S':
						if err := saveBuffer(fname, buf); err != nil {
							must(win.printAtBottom(err.Error()))
						} else {
							return
						}
					case 'd', 'D':
						return
					default:
						win.needsRedraw = true
				}
			} else {
				win.needsRedraw = true
			}
			/*if m, err := os.Stdin.Read(b[:]); err == nil {
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
					default:
						win.needsRedraw = true
					}
				}
			}*/
		case len(c) == 1 && c[0] == '\x7f':
			win.backspace()
		case len(c) == 1 && c[0] == '\f':
			must(win.printAtBottom("Go to line: "))
			lineStr, err := rawGetLine(inputCh, win.w)
			must(err)
			y, err := strconv.ParseInt(lineStr, 10, 32)
			if err == nil {
				win.gotoLine(int(y - 1))
			}
		case len(c) == 1 && c[0] == 6:
			must(win.printAtBottom("Search: "))
			reText, err := rawGetLine(inputCh, win.w)
			must(err)
			re, err := regexp.Compile(reText)
			if err != nil {
				must(win.printAtBottom(err.Error()))
			} else {
				win.searchRegexp(re)
			}
		case len(c) > 0 && (c[0] >= ' ' || c[0] == '\r' || c[0] == '\t'):
			win.typeText(c)
		}
	}
}
