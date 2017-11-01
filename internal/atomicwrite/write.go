// Package atomicwrite provides functions to write files atomically.
package atomicwrite

import (
	"io"
	"io/ioutil"
	"os"

	"github.com/pkg/errors"
)

// Write atomically overwrites the file at filename with the content written by the
// given function.
// The file is created if it doesn't already exist.
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
