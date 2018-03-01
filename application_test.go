package main

import (
	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/termesc"
	"testing"

	"io"
	"io/ioutil"
	"os"
	"time"
)

const (
	stdWidth  = 40
	stdHeight = 20
)

func TestMouseEventsOutsidePrompt(t *testing.T) {
	app := &application{mainWindow: newWindow(stdWidth, stdHeight, buffer.New()), promptWindow: newWindow(stdWidth, 1, buffer.New()), width: stdWidth, height: stdHeight}
	app.handleMouseEvent(termesc.MouseEvent{X: 5, Y: 5, Move: true, Button: termesc.NoButton})
	if app.promptWindow == nil {
		t.Error("after mouse move outside prompt, prompt window was closed, shouldn't have been")
	}
	app.handleMouseEvent(termesc.MouseEvent{X: 6, Y: 6, Button: termesc.ReleaseButton})
	if app.promptWindow != nil {
		t.Error("after mouse click ouside prompt, prompt window wasn't closed, should have been")
	}
}

// An inactiveReader is a Reader that blocks arbitrarily long, then immediately yields EOF. This is used
// to test parts of the application's event loop in isolation.
type inactiveReader chan struct{}

func (r inactiveReader) Read(b []byte) (int, error) {
	<-r
	return 0, io.EOF
}

func TestAutoSave(t *testing.T) {
	f, err := ioutil.TempFile("", "mflg-auto-save-test")
	if err != nil {
		t.Fatal(err)
	}
	name := f.Name()
	f.Close()
	defer os.Remove(name)
	const saveDelay = time.Second / 20
	app := &application{width: stdWidth, height: stdHeight, filename: name, saveDelay: saveDelay}
	if err := app.navigateTo(name); err != nil {
		t.Fatal(err)
	}
	app.resize(stdWidth, stdHeight)
	fakeConsole := make(inactiveReader)
	go app.run(fakeConsole, nil, ioutil.Discard)
	typeString(app.mainWindow, "ABC")
	time.Sleep(2 * saveDelay)
	checkFileContents(t, name, "ABC")
	typeString(app.mainWindow, "\rBlorp")
	time.Sleep(2 * saveDelay)
	checkFileContents(t, name, "ABC\nBlorp")
	app.mainWindow.selection.Put(buffer.Range{point{0, 1}, point{3, 1}})
	app.mainWindow.backspace()
	time.Sleep(2 * saveDelay)
	checkFileContents(t, name, "ABC\nrp")
	close(fakeConsole)
}

func checkFileContents(t *testing.T, filename, want string) {
	t.Helper()
	data, err := ioutil.ReadFile(filename)
	if err != nil {
		t.Error(err)
	}
	if got := string(data); got != want {
		t.Errorf("ReadFile(%q): got %q, want %q", filename, got, want)
	}
}
