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
	crlf       = []byte("\r\n")
	tab        = []byte("\t")
	fourSpaces = []byte("    ")
)

func renderBufferAt(buf *buffer.Buffer, topLine int, window io.Writer, width, height int) error {
	lines := buf.SliceLines(topLine, topLine+height)
	const gutterSize = 4
	for i, line := range lines {
		if len(line) > 0 && line[len(line)-1] == '\n' {
			line = line[:len(line)-1]
		}
		line = truncateToWidth(bytes.Replace(line, tab, fourSpaces, -1), width-gutterSize)
		if _, err := fmt.Fprintf(window, "% 3d ", topLine+i+1); err != nil {
			return err
		}
		if _, err := window.Write(line); err != nil {
			return err
		}
		if i+1 < height {
			if _, err := window.Write(crlf); err != nil {
				return err
			}
		}
	}
	return nil
}

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
	defer terminal.Restore(int(os.Stdin.Fd()), oldMode)
	enterAlternateScreen()
	defer exitAlternateScreen()
	if err := renderBufferAt(buf, 0, os.Stdout, w, h); err != nil {
		panic(err)
	}
	for {
		var b [5]byte
		n, err := os.Stdin.Read(b[:])
		if err != nil || (n == 1 && b[0] == 'q') {
			break
		}
		fmt.Printf("%q\r\n", b[:n])
	}
}
