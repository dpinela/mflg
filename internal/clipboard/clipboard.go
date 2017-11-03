// Package clipboard provides functions for copying and pasting text
// across different mflg instances running for the same user.
//
// On macOS, this uses the system clipboard and thus works across all applications.
package clipboard

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/dpinela/mflg/internal/atomicwrite"
	"github.com/pkg/errors"
)

// Copy overwrites the clipboard's contents with the given data.
func Copy(data []byte) error {
	return copyGeneric(data)
}

// Paste returns the last data stored with Copy by any instance of mflg of the same user,
// or the last data copied into the system clipboard if that is supported.
func Paste() ([]byte, error) {
	return pasteGeneric()
}

func homeDirectory() (string, error) {
	if d, ok := os.LookupEnv("HOME"); ok {
		return d, nil
	}
	if d, ok := os.LookupEnv("USERPROFILE"); ok {
		return d, nil
	}
	return "", errors.New("home directory not found")
}

func mflgPath(elems ...string) (string, error) {
	h, err := homeDirectory()
	if err != nil {
		return "", err
	}
	// This would be more readable if we could do filepath.Join(h, ".mflg", elems...)
	return filepath.Join(append([]string{h, ".mflg"}, elems...)...), nil
}

func mkMflgPath(elems ...string) (string, error) {
	p, err := mflgPath(elems...)
	if err != nil {
		return p, err
	}
	return p, os.MkdirAll(p, 0700)
}

const genericClipboardFile = "clipboard"

func copyGeneric(data []byte) error {
	const errString = "copy failed"

	p, err := mkMflgPath()
	if err != nil {
		return errors.Wrap(err, errString)
	}
	err = atomicwrite.Write(filepath.Join(p, genericClipboardFile), func(w io.Writer) error { _, err := w.Write(data); return err })
	return errors.Wrap(err, errString)
}

func pasteGeneric() ([]byte, error) {
	const errString = "paste failed"
	p, err := mflgPath(genericClipboardFile)
	if err != nil {
		return nil, errors.Wrap(err, errString)
	}
	data, err := ioutil.ReadFile(p)
	return data, errors.Wrap(err, errString)
}
