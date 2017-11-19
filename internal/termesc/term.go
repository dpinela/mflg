// Package termesc abstracts terminal ANSI escape codes.
package termesc

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"time"
)

const csi = "\x1B["

const (
	ClearScreen          = csi + "2J"     // Clears the entire visible area of the console
	ClearLine            = csi + "2K"     // Clears the line the cursor is on
	EnterAlternateScreen = csi + "?1049h" // Switches to the alternate screen
	ExitAlternateScreen  = csi + "?1049l" // Switches from the alternate screen to the regular one

	EnableMouseReporting  = csi + "?1000h" + csi + "?1015h" // Causes mouse escape sequences to be sent to the application when mouse events occur
	DisableMouseReporting = csi + "?1015l" + csi + "?1000l" // Restores the console's default mouse handling

	ResetGraphicAttributes = csi + "m"
	Bold = csi + "1m"
	ReverseVideo           = csi + "7m"

	UpKey    = csi + "A"
	DownKey  = csi + "B"
	LeftKey  = csi + "D"
	RightKey = csi + "C"
)

// SetCursorPos returns a code that sets the cursor's position to (y, x).
// Coordinates are 1-based.
func SetCursorPos(y, x int) string { return fmt.Sprintf(csi+"%d;%dH", y, x) }

// ConsoleReader provides an interface for reading console input in discrete units
// of runes and terminal escape codes.
// It is not safe for concurrent use.
type ConsoleReader struct {
	r           *bufio.Reader
	pendingPeek chan peekRes
}

// NewConsoleReader returns a new ConsoleReader which reads from r.
func NewConsoleReader(r io.Reader) *ConsoleReader {
	// Console input comes (to a computer) infrequently and in small amounts,
	// so a small buffer suffices.
	return &ConsoleReader{r: bufio.NewReaderSize(r, 64)}
}

// Since ESC both appears as the representation of the ESC key and as a prefix to escape codes
// representing mouse events and other keys, we need to wait for some time after reading ESC
// to see if it's the former or the latter. This delay should be large enough for the computer
// and imperceptible for a human.
// This also applies to parsing of sequences like ESC O P (F1) and ESC b (Alt+Left).
const escDelay = 10 * time.Millisecond

// ReadToken reads and returns a complete UTF-8 encoded rune or escape sequence.
func (r *ConsoleReader) ReadToken() (string, error) {
	r.waitPendingPeek()
	c, _, err := r.r.ReadRune()
	if err != nil {
		return "", err
	}
	if c != 0x1B {
		return string(c), nil
	}
	token := make([]byte, 0, 16)
	if nextB, err := r.peekTimeout(1, escDelay); err == nil && nextB[0] == '[' {
		// No need to wait for the peek to finish; we know it already did, so we can
		// safely read without triggering a race.
		r.r.Discard(1)
		token = append(token, 0x1B, '[')
		for {
			b, err := r.r.ReadByte()
			if err != nil {
				return string(token), err
			}
			// Old xterm-style (DECSET 1000 alone) mouse escape
			if len(token) == 2 && b == 'M' {
				token = append(token, 'M', 0, 0, 0)
				_, err = io.ReadFull(r.r, token[3:])
				return string(token), err
			}
			token = append(token, b)
			if b >= 0x40 && b < 0x7F {
				return string(token), nil
			}
		}
	} else {
		return string(c), nil
	}
}

var errTimedOut = errors.New("peek: timed out")

type peekRes struct {
	data []byte
	err  error
}

// If a peek operation is pending, wait for it to finish.
func (r *ConsoleReader) waitPendingPeek() {
	if r.pendingPeek != nil {
		<-r.pendingPeek
		r.pendingPeek = nil
	}
}

// peekTimeout returns the n next bytes from the input without consuming them,
// subject to a timeout dt.
// If it times out, you must call waitPendingPeek() before the next operation on the
// bufio.Reader.
func (r *ConsoleReader) peekTimeout(n int, dt time.Duration) ([]byte, error) {
	if r.r.Buffered() >= n {
		return r.r.Peek(n)
	}
	// Make this an async channel so that if the timeout fires, the peeking goroutine
	// doesn't block and leak.
	readyCh := make(chan peekRes, 1)
	go func() {
		data, err := r.r.Peek(n)
		readyCh <- peekRes{data, err}
	}()
	select {
	case pr := <-readyCh:
		return pr.data, pr.err
	case <-time.After(dt):
		r.pendingPeek = readyCh
		return nil, errTimedOut
	}
}
