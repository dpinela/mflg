// Package atomicwrite provides functions to write files atomically.
package atomicwrite

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
)

const defaultPerms = os.FileMode(0640)

// Write atomically overwrites the file at filename with the content written by the
// given function.
// The file is created with mode 0640 if it doesn't already exist; if it does, its permissions will be
// preserved if possible.
func Write(filename string, contentWriter func(io.Writer) error) error {
	tf, err := ioutil.TempFile("", "mflg-atomic-write")
	if err != nil {
		return errors.Wrap(err, errString(filename))
	}
	name := tf.Name()
	if err = contentWriter(tf); err != nil {
		os.Remove(name)
		tf.Close()
		return errors.Wrap(err, errString(filename))
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
		return errors.Wrap(err, errString(filename))
	}
	if err = os.Rename(name, filename); err != nil {
		os.Remove(name)
		return errors.Wrap(err, errString(filename))
	}
	return nil
}

func errString(filename string) string { return "atomic write to " + filename + " failed" }
