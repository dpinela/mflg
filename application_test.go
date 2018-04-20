package main

import (
	"github.com/dpinela/mflg/internal/buffer"
	"github.com/dpinela/mflg/internal/config"
	"github.com/dpinela/mflg/internal/termesc"
	"testing"

	"io"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

const (
	stdWidth  = 40
	stdHeight = 20
)

var ellipsifyTests = []struct {
	in    string
	width int
	out   string
}{
	{"érrôr writing lápis.txt", 23, "érrôr writing lápis.txt"},
	{"érrôr wrîting lápis.txt", 10, "érrôr w..."},
	{"error", 2, ".."},
	{"error", 1, "."},
	{"error", 0, ""},
	{"er", 3, "er"},
	{"er", 2, "er"},
}

func TestEllipsify(t *testing.T) {
	for _, tt := range ellipsifyTests {
		if got := ellipsify(tt.in, tt.width); got != tt.out {
			t.Errorf("ellipsify(%q, %d) = %q; want %q", tt.in, tt.width, got, tt.out)
		}
	}
}

func TestMouseEventsOutsidePrompt(t *testing.T) {
	app := &application{mainWindow: newWindow(stdWidth, stdHeight, buffer.New(), 4), promptWindow: newWindow(stdWidth, 1, buffer.New(), 4), width: stdWidth, height: stdHeight}
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
	app := &application{width: stdWidth, height: stdHeight, saveDelay: saveDelay, config: &config.Config{TabWidth: 4}}
	if err := app.navigateTo(name); err != nil {
		t.Fatal(err)
	}
	app.resize(stdWidth, stdHeight)
	fakeConsole := make(inactiveReader)
	defer close(fakeConsole)
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
}

func TestAutoReload(t *testing.T) {
	dir, err := ioutil.TempDir("", "mflg-auto-reload-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	app := &application{width: stdWidth, height: stdHeight, config: &config.Config{TabWidth: 2}, taskQueue: make(chan func(), 8)}
	fakeConsole := make(inactiveReader)
	defer close(fakeConsole)
	done := make(chan struct{}, 1)
	fileA := filepath.Join(dir, "A")
	f, err := os.Create(fileA)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	f.WriteString("Hello.")
	if err := app.navigateTo(f.Name()); err != nil {
		t.Fatal(err)
	}
	go app.run(fakeConsole, nil, ioutil.Discard)
	t.Run("OnWrite", func(t *testing.T) {
		f.WriteString(" LOOK, NEW TEXT!")
		time.Sleep(30 * time.Millisecond)
		app.do(func() {
			checkLineContent(t, 1, app.mainWindow, 0, "Hello. LOOK, NEW TEXT!")
			done <- struct{}{}
		})
		<-done
	})
	t.Run("OnDelete", func(t *testing.T) {
		os.Remove(fileA)
		time.Sleep(200 * time.Millisecond)
		app.do(func() {
			checkLineContent(t, 2, app.mainWindow, 0, "")
			if app.mainWindow.buf.LineCount() != 1 {
				t.Error("after delete, ", fileA, " is not empty in-editor")
			}
			done <- struct{}{}
		})
		<-done
	})
	t.Run("OnCreate", func(t *testing.T) {
		const surprise = "Surprise!!!"
		nameB := filepath.Join(dir, "B")
		if err := app.navigateTo(nameB); err != nil {
			t.Fatal(err)
		}
		ioutil.WriteFile(nameB, []byte(surprise), 0644)
		time.Sleep(30 * time.Millisecond)
		app.do(func() {
			checkLineContent(t, 3, app.mainWindow, 0, surprise)
			done <- struct{}{}
		})
		<-done
	})
	nameC := filepath.Join(dir, "Animals", "Felidae", "cat")
	t.Run("OnParentDirCreate", func(t *testing.T) {
		const greeting = "meow"
		if err := app.navigateTo(nameC); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(nameC), 0755); err != nil {
			t.Error(err)
		}
		ioutil.WriteFile(nameC, []byte(greeting), 0644)
		time.Sleep(30 * time.Millisecond)
		app.do(func() {
			checkLineContent(t, 4, app.mainWindow, 0, greeting)
			done <- struct{}{}
		})
		<-done
	})
	t.Run("OnParentDirDelete", func(t *testing.T) {
		app.do(func() {
			// Make sure the file is loaded, even if the previous test failed
			app.reloadFile()
			done <- struct{}{}
		})
		<-done
		if err := os.RemoveAll(filepath.Dir(nameC)); err != nil {
			t.Error(err)
		}
		time.Sleep(30 * time.Millisecond)
		app.do(func() {
			checkLineContent(t, 5, app.mainWindow, 0, "")
			if app.mainWindow.buf.LineCount() != 1 {
				t.Error("after delete, ", nameC, " is not empty in-editor")
			}
			done <- struct{}{}
		})
		<-done
	})
}

func TestNavigation(t *testing.T) {
	d, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatal(err)
	}
	if strings.IndexByte(d, ':') != -1 || filepath.Separator == ':' {
		t.Fatal("generated file names will contain colons; some navigation syntax is ambiguous in this case")
	}
	nameA := filepath.Join(d, "A")
	nameB := filepath.Join(d, "B")
	app := &application{width: stdWidth, height: stdHeight, config: &config.Config{TabWidth: 4}}
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
		app.mainWindow.moveCursorRight()
		app.testNav(t, nameA+":2")
		app.checkLocation(t, nameA, 1)
	})
	t.Run("Back", func(t *testing.T) {
		app.testBack(t)
		app.checkFullLocation(t, nameB, point{X: 1, Y: 0})
		app.testBack(t)
		app.checkLocation(t, nameA, 0)
		// Avoid passing this last test by coincidence; the location before this sub-test is nameA:1
		app.testBack(t)
		app.testBack(t)
		app.checkLocation(t, nameA, 2)
	})
	t.Run("DifferentFilesRelative", func(t *testing.T) {
		app.testNav(t, "A:2")
		app.checkLocation(t, nameA, 1)
		nameC := filepath.Join("X", "B")
		app.testNav(t, nameC)
		app.checkLocation(t, filepath.Join(d, nameC), 0)
		app.testNav(t, filepath.Join("..", "A"))
		app.checkLocation(t, nameA, 0)
	})
	t.Run("ShellFilenameExpansion", func(t *testing.T) {
		defer func(old func() (*user.User, error)) { currentUser = old }(currentUser)
		currentUser = func() (*user.User, error) { return &user.User{HomeDir: d}, nil }
		if err := os.Setenv("NEW_FILE", "C"); err != nil {
			t.Error(err)
		}
		app.testNav(t, "~/$NEW_FILE")
		app.checkLocation(t, filepath.Join(d, "C"), 0)
	})
	t.Run("CycleRegexMatches", func(t *testing.T) {
		app.testNav(t, "B:[ae]t$")
		app.checkLocation(t, nameB, 1)
		app.gotoNextMatch()
		app.checkLocation(t, nameB, 2)
		app.gotoNextMatch()
		app.checkLocation(t, nameB, 4)
		app.testBack(t)
		app.checkLocation(t, nameB, 2)
		app.gotoNextMatch()
		app.checkLocation(t, nameB, 4)
		app.gotoNextMatch()
		app.checkLocation(t, nameB, 1)
		app.mainWindow.cursorPos = point{X: 1, Y: 3}
		app.gotoNextMatch()
		app.checkLocation(t, nameB, 4)
	})
}

func (app *application) testNav(t *testing.T, dest string) {
	t.Helper()
	if err := app.navigateTo(dest); err != nil {
		t.Error(err)
	}
}

func (app *application) testBack(t *testing.T) {
	t.Helper()
	if err := app.back(); err != nil {
		t.Error(err)
	}
}

func (app *application) checkLocation(t *testing.T, filename string, lineNum int) {
	t.Helper()
	app.checkFullLocation(t, filename, buffer.Point{X: 0, Y: lineNum})
}

func (app *application) checkFullLocation(t *testing.T, filename string, wantTP buffer.Point) {
	t.Helper()
	tp := app.mainWindow.windowCoordsToTextCoords(app.mainWindow.cursorPos)
	if app.currentFile() != filename || tp != wantTP {
		t.Errorf("editor at %s:%v, want %[1]s:%[3]v", app.currentFile(), tp, wantTP)
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
