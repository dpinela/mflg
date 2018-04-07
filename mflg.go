package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/dpinela/mflg/internal/atomicwrite"
	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/config"
	"github.com/dpinela/mflg/internal/termesc"

	"github.com/pkg/errors"
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
	w, h, err := terminal.GetSize(0)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error finding terminal size:", err)
		os.Exit(1)
	}
	conf, err := config.Load()
	if err != nil && !os.IsNotExist(errors.Cause(err)) {
		fmt.Fprintln(os.Stderr, err)
	}
	app := application{saveDelay: 1 * time.Second, width: w, height: h, cursorVisible: true, config: conf, taskQueue: make(chan func(), 16)}
	if err := app.navigateTo(os.Args[1]); err != nil {
		fmt.Fprintf(os.Stderr, "error loading %s: %v", os.Args[1], err)
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
	if err := app.run(os.Stdin, resizeCh, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
