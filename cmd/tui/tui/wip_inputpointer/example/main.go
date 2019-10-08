package main

import (
	"time"

	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/inputpointer"
)

var _str = "old string"

func main() {
	var str = &_str

	go func() {
		for t := range time.NewTicker(time.Second / 2).C {
			_str = t.String()
			tview.Draw()
		}
	}()

	s := inputpointer.NewString(str)
	s.SetLabel("Watch as I update")

	tview.Initialize()
	tview.SetRoot(s, true)
	tview.SetFocus(s)

	if err := tview.Run(); err != nil {
		panic(err)
	}
}
