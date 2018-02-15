package atomicwrite

import (
	"bytes"
	"io"
	"io/ioutil"
	"testing"
	"os"
	"path/filepath"
)

var testContent = []byte("lorem ipsum\ndolor $it amet\nmet cons€quiat\neladamet")

func fatalErr(t *testing.T, err error) {
	if err != nil {
		t.Fatal(err)
	}
}

func TestWrite(t *testing.T) {
	tf, err := ioutil.TempFile("", "atomicwrite-test")
	name := tf.Name()
	fatalErr(t, err)
	fatalErr(t, tf.Close())
	if err = Write(name, func(w io.Writer) error { _, err := w.Write(testContent); return err }); err != nil {
		t.Error(err)
	}
	data, err := ioutil.ReadFile(name)
	if err != nil {
		t.Error(err)
	}
	if !bytes.Equal(data, testContent) {
		t.Errorf("read back written data: got %q, want %q", data, testContent)
	}
}

func TestPermissionsPreserved(t *testing.T) {
	td, err := ioutil.TempDir("", "atomicwrite-testdir")
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Join(td, "token")
	f, err := os.OpenFile(name, os.O_WRONLY | os.O_CREATE | os.O_EXCL, 0755)
	if err != nil {
		t.Fatal(err)
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	oldPerms := info.Mode() & os.ModePerm
	f.Close()
	if err := Write(name, func(w io.Writer) error { _, err := w.Write(testContent); return err }); err != nil {
		t.Error(err)
	}
	if info, err = os.Stat(name); err != nil {
		t.Fatal(err)
	}
	if newPerms := info.Mode() & os.ModePerm; newPerms != oldPerms {
		t.Errorf("after Write, got permissions %v, want %v", newPerms, oldPerms)
	}
}