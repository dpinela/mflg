package clipboard

import (
	"bytes"
	"testing"
)

var testData = []byte("Som€ copypasta")

func check(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Error(err)
	}
}

func TestCopyPaste(t *testing.T) {
	check(t, Copy(testData))
	data, err := Paste()
	check(t, err)
	if !bytes.Equal(data, testData) {
		t.Errorf("after copy and paste: got %q, want %q", data, testData)
	}
}
