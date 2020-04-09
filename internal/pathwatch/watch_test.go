package pathwatch

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func init() {
	notifyOnAdd = true
}

func TestWatch(t *testing.T) {
	dir, err := ioutil.TempDir("", "mflg-path-watch-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	w := NewWatcher()
	t.Run("OnWrite", func(t *testing.T) {
		f := create(t, filepath.Join(dir, "A"))
		changes := w.addWait(f.Name())
		f.WriteString("Hello.")
		f.Close()
		waitChange(t, changes, 250*time.Millisecond)
	})
	t.Run("OnDelete", func(t *testing.T) {
		f := create(t, filepath.Join(dir, "B"))
		changes := w.addWait(f.Name())
		f.Close()
		os.Remove(f.Name())
		waitChange(t, changes, 250*time.Millisecond)
	})
	t.Run("OnCreate", func(t *testing.T) {
		name := filepath.Join(dir, "C")
		changes := w.addWait(name)
		create(t, name).Close()
		waitChange(t, changes, 250*time.Millisecond)
	})
	t.Run("OnParentDirCreate", func(t *testing.T) {
		name := filepath.Join(dir, "D", "E")
		changes := w.addWait(name)
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		create(t, name).Close()
		waitChange(t, changes, 250*time.Millisecond)
	})
	t.Run("OnParentDirDelete", func(t *testing.T) {
		name := filepath.Join(dir, "F", "G")
		if err := os.MkdirAll(filepath.Dir(name), 0700); err != nil {
			t.Fatal(err)
		}
		create(t, name).Close()
		changes := w.addWait(name)
		os.RemoveAll(filepath.Dir(name))
		waitChange(t, changes, 250*time.Millisecond)
	})
}

func (w *Watcher) addWait(path string) <-chan struct{} {
	changes := make(chan struct{}, 10)
	w.Add(path, changes)
	<-changes
	return changes
}

func create(t *testing.T, path string) *os.File {
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func waitChange(t *testing.T, ch <-chan struct{}, timeout time.Duration) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(timeout):
		t.Error("failed to receive notification after", timeout)
	}
}
