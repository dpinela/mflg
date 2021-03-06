package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/dpinela/mflg/internal/atomicwrite"
	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/termdraw"
	"github.com/dpinela/mflg/internal/termesc"

	"golang.org/x/crypto/ssh/terminal"
	"golang.org/x/sys/unix"
)

func saveBuffer(fname string, buf *buffer.Buffer) error {
	if fname == os.DevNull {
		return nil
	}
	return atomicwrite.Write(fname, func(w io.Writer) error { _, err := buf.WriteTo(w); return err })
}

func allASCIIDigits(s string) bool {
	for i := range s {
		if !(s[i] >= '0' && s[i] <= '9') {
			return false
		}
	}
	return true
}

func newScratchFile() (name string, err error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("error creating scratch file: %w", err)
	}
	return filepath.Join(dir, "mflg", "scratch "+time.Now().Format("2006-01-02 15.04.05.0")), nil
}

// A mflg instance is made of three components:
// - a main window, which handles text editing for the open file
// - a prompt window, which provides the same functionality for the text entered in response
//   to various command prompts.
// - an application object, which coordinates rendering of these two windows, and distributes input
//   between them as appropriate.

func main() {
	var selector string
	if len(os.Args) < 2 {
		name, err := newScratchFile()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "saving buffer to", name)
		selector = name
	} else {
		selector = os.Args[1]
	}
	w, h, err := terminal.GetSize(0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error finding terminal size:", err)
		os.Exit(1)
	}
	app := newApplication(os.Stdout, termdraw.Point{X: w, Y: h})
	defer app.fsWatcher.Close()
	app.loadConfig()
	if err := app.navigateTo(selector); err != nil {
		fmt.Fprintf(os.Stderr, "error loading %s: %v\n", os.Args[1], err)
		os.Exit(1)
	}
	oldMode, err := terminal.MakeRaw(0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error entering raw mode:", err)
		os.Exit(1)
	}
	defer terminal.Restore(0, oldMode)
	os.Stdout.WriteString(termesc.EnableMouseReporting + termesc.EnableBracketedPaste + termesc.EnterAlternateScreen)
	defer os.Stdout.WriteString(termesc.ExitAlternateScreen + termesc.DisableBracketedPaste + termesc.ShowCursor + termesc.DisableMouseReporting)
	resizeCh := make(chan os.Signal, 32)
	signal.Notify(resizeCh, unix.SIGWINCH)
	if err := app.run(os.Stdin, resizeCh); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
