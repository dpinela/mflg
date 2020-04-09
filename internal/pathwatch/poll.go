// Package pathwatch provides file system change notifications.
package pathwatch

import (
	"os"
	"time"
)

// A Watcher keeps track of a set of paths and sends notifications on user-provided
// channels whenever the file or directory at one of them changes in any way.
// The specific nature of the change is not reported; it is up to the user to determine
// what happened.
//
// Any errors that the Watcher encounters while monitoring the paths are delivered on the
// channel returned by Error.
type Watcher struct {
	files   map[string]*watchedFile
	errors  chan error
	control chan func()
}

type watchedFile struct {
	lastInfo  os.FileInfo
	observers []chan<- struct{}
}

// NewWatcher starts a new watcher.
// When no longer in use, the user should call Close to release resources associated with it.
func NewWatcher() *Watcher {
	w := &Watcher{files: map[string]*watchedFile{}, errors: make(chan error, 10), control: make(chan func(), 10)}
	go w.run()
	return w
}

// Normally we don't want a notification when we add a file, since it's redundant,
// but for testing we need it in order to be able to reliably detect modifications without
// races.
var notifyOnAdd = false

// Add begins sending change notifications for a path on the given channel.
// Multiple calls to Add for the same path, but different channels, are permitted;
// in that case, the notifications will be sent on all of them.
func (w *Watcher) Add(path string, ch chan<- struct{}) {
	w.control <- func() {
		wf, ok := w.files[path]
		if !ok {
			info, err := os.Stat(path)
			if err != nil && !os.IsNotExist(err) {
				w.errors <- err
			}
			wf = &watchedFile{lastInfo: info}
			w.files[path] = wf
		}
		wf.observers = append(wf.observers, ch)
		if notifyOnAdd {
			ch <- struct{}{}
		}
	}
}

// Remove stops sending change notifications for a path on the given channel.
// It does not cancel other calls to Add made for the same path, but different
// channels.
func (w *Watcher) Remove(path string, ch chan<- struct{}) {
	w.control <- func() {
		wf, ok := w.files[path]
		if !ok {
			return
		}
		for i, ob := range wf.observers {
			if ob != ch {
				continue
			}
			if len(wf.observers) == 1 {
				delete(w.files, path)
			} else {
				n := len(wf.observers) - 1
				wf.observers[i] = wf.observers[n]
				wf.observers = wf.observers[:n]
			}
			return
		}
	}
}

// Errors returns a channel on which the Watcher delivers errors it encounters.
func (w *Watcher) Errors() <-chan error { return w.errors }

// Close stops delivering change notifications for any paths and releases all resources
// associated with the watcher.
func (w *Watcher) Close() { w.control <- nil }

func (w *Watcher) run() {
	tick := time.NewTicker(time.Second / 8)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
			for path, wf := range w.files {
				info, err := os.Stat(path)
				if err != nil && !os.IsNotExist(err) {
					w.errors <- err
					continue
				}
				if !fileInfoEqual(wf.lastInfo, info) {
					for _, ob := range wf.observers {
						ob <- struct{}{}
					}
					wf.lastInfo = info
				}
			}
		case f := <-w.control:
			if f == nil {
				return
			}
			f()
		}
	}
}

func fileInfoEqual(a, b os.FileInfo) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.ModTime().Equal(b.ModTime()) && a.Size() == b.Size()
}
