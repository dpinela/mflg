package main

import (
	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/termesc"
	"testing"

	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
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
	app := &application{width: stdWidth, height: stdHeight, saveDelay: saveDelay}
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

func TestNavigation(t *testing.T) {
	d, err := ioutil.TempDir("", "mflg-nav-test")
	if err != nil {
		t.Fatal(err)
	}
	if d, err = filepath.Abs(d); err != nil {
		t.Fatal(err)
	}
	if strings.IndexByte(d, ':') != -1 || filepath.Separator == ':' {
		t.Fatal("generated file names will contain colons; some navigation syntax is ambiguous in this case")
	}
	defer os.RemoveAll(d)
	nameA := filepath.Join(d, "A")
	nameB := filepath.Join(d, "B")
	putFile(t, nameA, []byte("lorem\nipsum\n"))
	putFile(t, nameB, []byte("sit\namet\nconsequiat"))
	app := &application{width: stdWidth, height: stdHeight}
	t.Run("Start", func(t *testing.T) {
		app.testNav(t, nameA)
		app.checkLocation(t, nameA, 0)
	})
	t.Run("SameFile", func(t *testing.T) {
		app.testNav(t, ":3")
		app.checkLocation(t, nameA, 2)

		app.testNav(t, ":^.psu")
		app.checkLocation(t, nameA, 1)
		app.testNav(t, nameA+":1")
		app.checkLocation(t, nameA, 0)
	})
	t.Run("DifferentFiles", func(t *testing.T) {
		app.testNav(t, nameB)
		app.checkLocation(t, nameB, 0)
		app.testNav(t, nameA+":2")
		app.checkLocation(t, nameA, 1)
	})
}

func (app *application) testNav(t *testing.T, dest string) {
	t.Helper()
	if err := app.navigateTo(dest); err != nil {
		t.Error(err)
	}
}

func (app *application) checkLocation(t *testing.T, filename string, lineNum int) {
	t.Helper()
	y := app.mainWindow.windowCoordsToTextCoords(app.mainWindow.cursorPos).Y
	if app.currentFile() != filename || y != lineNum {
		t.Log(app.mainWindow.topLine, app.mainWindow.cursorPos)
		t.Errorf("editor at %s:%d, want %s:%d", app.currentFile(), y, filename, lineNum)
	}
}

func putFile(t *testing.T, name string, content []byte) {
	t.Helper()
	if err := ioutil.WriteFile(name, content, 0600); err != nil {
		t.Fatal(err)
	}
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
