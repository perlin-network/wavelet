package errdialog

import (
	"fmt"
	"strings"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/clearbg"
)

type DialogSettings struct {
	FgColor tcell.Color
	BgColor tcell.Color

	ButtonFgColor tcell.Color
	ButtonBgColor tcell.Color

	WordWrapped      bool
	WordWrappedWidth int
	ClearBackground  bool
}

// DefaultSettings is the default settings to use when nil is given.
var DefaultSettings = &DialogSettings{
	FgColor: tcell.ColorWhite,
	BgColor: tcell.ColorRed,

	ButtonFgColor: tcell.ColorBlack,
	ButtonBgColor: tcell.ColorWhite,

	WordWrapped:      true,
	WordWrappedWidth: 40,
	ClearBackground:  true,
}

// CallDialogF calls the dialog with a Sprintf.
func CallDialogF(f string, i ...interface{}) {
	CallDialog(fmt.Sprintf(f, i...), nil)
}

// CallDialog shows a dialog.
func CallDialog(text string, settings *DialogSettings) {
	if settings == nil {
		settings = DefaultSettings
	}

	if !settings.ClearBackground {
		// Turn off clearing the background
		clearbg.Paused = true
		defer clearbg.Enable()
	}

	// Save the old primitive
	oldPrimitive := tview.GetRoot()
	defer tview.SetRoot(oldPrimitive, true)
	defer tview.SetFocus(oldPrimitive)

	finish := make(chan struct{})

	modal := tview.NewModal()

	modal.SetBackgroundColor(tcell.ColorDefault)
	modal.Frame.SetBackgroundColor(settings.BgColor)

	modal.Form.SetBackgroundColor(settings.BgColor)
	modal.Form.SetFieldBackgroundColor(settings.BgColor)
	modal.Form.SetLabelColor(settings.FgColor)
	modal.Form.SetFieldTextColor(settings.FgColor)

	modal.Form.SetButtonTextColor(settings.ButtonFgColor)
	modal.Form.SetButtonBackgroundColor(settings.ButtonBgColor)

	modal.AddButtons([]string{"Ok"})
	modal.SetDoneFunc(func(_ int, _ string) {
		finish <- struct{}{}
	})

	modal.SetText(
		strings.Join(tview.WordWrap(text, 40), "\n"),
	)

	// Set the root primitive
	tview.SetRoot(modal, true)
	tview.SetFocus(modal)

	tview.Draw()

	<-finish
}
