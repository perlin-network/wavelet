package main

import (
	"time"

	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/errdialog"
)

func main() {
	tview.Initialize()

	tv := tview.NewTextView()
	tv.SetText("Get ready in 3 seconds...")

	tview.SetRoot(tv, true)
	tview.SetFocus(tv)

	go func() {
		time.Sleep(3 * time.Second)
		errdialog.CallDialog("Warning! Warning!", nil)

		tv.SetText("mendokusai\noniichan yattoite")

		tview.Stop()
	}()

	if err := tview.Run(); err != nil {
		panic(err)
	}
}
