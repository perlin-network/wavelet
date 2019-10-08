// Package clearbg calls tview functions to make the background color match
// the terminal's.
package clearbg

import (
	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
)

func init() {
	tview.Initialize()

	tview.Styles.PrimitiveBackgroundColor = -1
	tview.SetBeforeDrawFunc(func(s tcell.Screen) bool {
		if BeforeDrawFunc(s) {
			return true
		}

		if !Paused {
			// Clear the leftover runes
			s.Clear()
		}

		return false
	})
}

// BeforeDrawFunc is a replacement callback. Packages should use this instead
// of tview's SetBeforeDrawFunc function.
var BeforeDrawFunc = func(s tcell.Screen) bool {
	return false
}

// Paused if true will not clear the screen. This is useful to make a toaster
// notification or dialog of some sort.
var Paused bool

// Enable turns off the Paused boolean. Made for cleaner deferring.
func Enable() {
	Paused = false
}
