package atomicwrite

import (
	"bytes"
	"io"
	"io/ioutil"
	"testing"
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
