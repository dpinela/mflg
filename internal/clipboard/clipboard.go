// Package clipboard provides functions for copying and pasting text
// across different mflg instances running for the same user.
//
// On macOS, this uses the system clipboard and thus works across all applications.
package clipboard

import (
	"bytes"
	"io"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"runtime"

	"github.com/dpinela/mflg/internal/atomicwrite"
	"github.com/pkg/errors"

	"github.com/tajtiattila/basedir"
)

// Copy overwrites the clipboard's contents with the given data.
func Copy(data []byte) error {
	return errors.WithMessage(copyGeneric(data), "copy failed")
}

// Paste returns the last data stored with Copy by any instance of mflg of the same user,
// or the last data copied into the system clipboard if that is supported.
func Paste() ([]byte, error) {
	data, err := pasteGeneric()
	return data, errors.WithMessage(err, "paste failed")
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

func clipboardFilename() (string, error) {
	mflgDir, err := basedir.Data.EnsureDir("mflg", 0700)
	if err != nil {
		return "", err
	}
	return filepath.Join(mflgDir, "clipboard"), nil
}

func copyBuiltin(data []byte) error {
	p, err := clipboardFilename()
	if err != nil {
		return err
	}
	return atomicwrite.Write(p, func(w io.Writer) error { _, err := w.Write(data); return err })
}

func pasteBuiltin() ([]byte, error) {
	p, err := clipboardFilename()
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
