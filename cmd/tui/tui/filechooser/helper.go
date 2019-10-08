package filechooser

import (
	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/errdialog"
)

// Spawn opens a file chooser and blocks until the user chooses a file, then
// return that file.
func Spawn() string {
	oldPrimitive := tview.GetRoot()
	defer tview.SetRoot(oldPrimitive, true)
	defer tview.SetFocus(oldPrimitive)

	selected := make(chan string)

	c := New()
	c.FilePick = func(filename string) {
		selected <- filename
	}

	c.SetDoneFunc(func() {
		selected <- ""
	})

	if err := c.ChangeDirectory(""); err != nil {
		go errdialog.CallDialog(err.Error(), nil)
		return ""
	}

	tview.SetRoot(c, true)
	tview.SetFocus(c)

	tview.Draw()

	return <-selected
}
