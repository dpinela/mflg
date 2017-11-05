// Package clipboard provides functions for copying and pasting text
// across different mflg instances running for the same user.
//
// On macOS, this uses the system clipboard and thus works across all applications.
package clipboard

import (
	"bytes"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/dpinela/mflg/internal/atomicwrite"
	"github.com/pkg/errors"
)

// Copy overwrites the clipboard's contents with the given data.
func Copy(data []byte) error {
	return errors.Wrap(copyGeneric(data), "copy failed")
}

// Paste returns the last data stored with Copy by any instance of mflg of the same user,
// or the last data copied into the system clipboard if that is supported.
func Paste() ([]byte, error) {
	data, err := pasteGeneric()
	return data, errors.Wrap(err, "paste failed")
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

func copyGeneric(data []byte) error {
	if runtime.GOOS == "darwin" {
		if err := copyToPasteboard(data); err == nil {
			return nil
		}
	}
	return copyBuiltin(data)
}

func pasteGeneric() ([]byte, error) {
	if runtime.GOOS == "darwin" {
		if data, err := pastePasteboard(); err == nil {
			return data, nil
		}
	}
	return pasteBuiltin()
}

const genericClipboardFile = "clipboard"

func copyBuiltin(data []byte) error {
	p, err := mkMflgPath()
	if err != nil {
		return err
	}
	return atomicwrite.Write(filepath.Join(p, genericClipboardFile), func(w io.Writer) error { _, err := w.Write(data); return err })
}

func pasteBuiltin() ([]byte, error) {
	p, err := mflgPath(genericClipboardFile)
	if err != nil {
		return nil, err
	}
	return ioutil.ReadFile(p)
}

func copyToPasteboard(b []byte) error {
	copyCmd := exec.Command("pbcopy")
	copyCmd.Stdin = bytes.NewReader(b)
	return copyCmd.Run()
}

func pastePasteboard() ([]byte, error) {
	return exec.Command("pbpaste").Output()
}
