//go:generate stringer -type=MouseButton

package termesc

import (
	"errors"
	"fmt"
)

// MouseEvent represents a mouse press, release or movement, or a scroll wheel tick.
type MouseEvent struct {
	Button              MouseButton // The mouse button that was pressed (if any)
	Shift, Alt, Control bool        // True if the corresponding modifier keys are held down
	Move                bool        // True if this is a mouse-move event, false if it is a press/release/scroll event.
	X, Y                int         // The viewport-space coordinates of the character the mouse was over
}

// MouseButton identifies the different mouse buttons. This includes both directions
// of the scroll wheel. It also includes the release-button event, which is identical
// for all buttons.
type MouseButton int8

// Identifiers for mouse buttons.
const (
	NoButton MouseButton = iota // denotes a mouse-move event
	LeftButton
	MiddleButton
	RightButton
	ReleaseButton
	ScrollUpButton
	ScrollDownButton
)

// Errors returned for invalid mouse escape sequences.
var (
	ErrNotAMouseEvent = errors.New("invalid format for mouse event")
	ErrInvalidCoords  = errors.New("mouse event coordinates are negative")
)

// ParseMouseEvent interprets a string as a mouse escape sequence and returns a MouseEvent
// describing its content.
// It accepts old xterm-style (DECSET 1000) as well as urxvt-style (DECSET 1000+1015)
// escape sequences.
func ParseMouseEvent(code string) (MouseEvent, error) {
	if len(code) == 6 && code[:3] == csi+"M" {
		return parseXtermMouseEvent(code)
	}
	return parseRxvtMouseEvent(code)
}

func parseRxvtMouseEvent(code string) (MouseEvent, error) {
	var ev MouseEvent
	var button byte
	if _, err := fmt.Sscanf(code, csi+"%d;%d;%dM", &button, &ev.X, &ev.Y); err != nil {
		return MouseEvent{}, ErrNotAMouseEvent
	}
	ev.setButtonInfo(button)
	ev.X--
	ev.Y--
	if ev.X < 0 || ev.Y < 0 {
		return ev, ErrInvalidCoords
	}
	return ev, nil
}

func parseXtermMouseEvent(code string) (MouseEvent, error) {
	var ev MouseEvent
	ev.setButtonInfo(code[3])
	ev.X = int(code[4]) - 33
	ev.Y = int(code[5]) - 33
	if ev.X < 0 || ev.Y < 0 {
		return ev, ErrInvalidCoords
	}
	return ev, nil
}

func (ev *MouseEvent) setButtonInfo(button byte) {
	ev.Shift = button&4 != 0
	ev.Alt = button&8 != 0
	ev.Control = button&0x10 != 0
	ev.Move = button&0x40 != 0 && button&0x20 == 0
	switch {
	case button&0x60 == 0x60:
		if button&1 != 0 {
			ev.Button = ScrollDownButton
		} else {
			ev.Button = ScrollUpButton
		}
	case button&0x40 != 0 && button&3 == 3:
		ev.Button = NoButton
	default:
		ev.Button = MouseButton((button & 3) + 1)
	}
}
