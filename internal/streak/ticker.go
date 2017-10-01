// Package streak tracks repeated occurrences of an event within some interval.
package streak

import "time"

// Tracker keeps track of the number of times a certain event has happened within
// a time interval T of each other. If it occurs longer than T after the last
// occurrence, the count is reset to 1.
//
// To initialize a Tracker, set its Interval field to that interval T.
type Tracker struct {
	ConsecutiveTicks int // The number of consecutive events so far.
	lastTickTime     time.Time
	Interval         time.Duration // The maximum time difference for two events to be considered consecutive.
}

// Tick records one occurrence of the event and returns the current tick count.
func (t *Tracker) Tick() int {
	now := time.Now()
	if t.lastTickTime.IsZero() || now.Sub(t.lastTickTime) > t.Interval {
		t.ConsecutiveTicks = 1
	} else {
		t.ConsecutiveTicks++
	}
	t.lastTickTime = now
	return t.ConsecutiveTicks
}
