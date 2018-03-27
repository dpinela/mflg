// Package atomicwrite provides functions to write files atomically.
package atomicwrite

import (
	"io"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/pkg/errors"
)

const (
	defaultPerms    = os.FileMode(0644)
	defaultDirPerms = os.FileMode(0755)
)

// Write atomically overwrites the file at filename with the content written by the
// given function.
// The file is created with mode 0644 if it doesn't already exist; if it does, its permissions will be
// preserved if possible.
// If some of the directories on the path don't already exist, they are created with mode 0755.
func Write(filename string, contentWriter func(io.Writer) error) error {
	tf, err := ioutil.TempFile("", "mflg-atomic-write")
	if err != nil {
		return errors.WithMessage(err, errString(filename))
	}
	name := tf.Name()
	if err = contentWriter(tf); err != nil {
		os.Remove(name)
		tf.Close()
		return errors.WithMessage(err, errString(filename))
	}
	// Keep existing file's permissions, when possible. This may race with a chmod() on the file.
	perms := defaultPerms
	if info, err := os.Stat(filename); err == nil {
		perms = info.Mode()
	}
	// It's better to save a file with the default TempFile permissions than not save at all, so if this fails we just carry on.
	tf.Chmod(perms)
	if err = tf.Close(); err != nil {
		os.Remove(name)
		return errors.WithMessage(err, errString(filename))
	}
	if err = os.MkdirAll(filepath.Dir(filename), defaultDirPerms); err != nil {
		os.Remove(name)
		return errors.WithMessage(err, errString(filename))
	}
	if err = os.Rename(name, filename); err != nil {
		os.Remove(name)
		return errors.WithMessage(err, errString(filename))
	}
	return nil
}

func errString(filename string) string { return "atomic write to " + filename + " failed" }
