// Package termesc abstracts terminal ANSI escape codes.
package termesc

import "fmt"

const csi = "\x1B["

const (
	ClearScreen          = csi + "2J"     // Clears the entire visible area of the console
	ClearLine            = csi + "2K"     // Clears the line the cursor is on
	EnterAlternateScreen = csi + "?1049h" // Switches to the alternate screen
	ExitAlternateScreen  = csi + "?1049l" // Switches from the alternate screen to the regular one

	UpKey    = csi + "A"
	DownKey  = csi + "B"
	LeftKey  = csi + "D"
	RightKey = csi + "C"
)

// SetCursorPos returns a code that sets the cursor's position to (y, x).
// Coordinates are 1-based.
func SetCursorPos(y, x int) string { return fmt.Sprintf(csi+"%d;%dH", y, x) }
