package main

import (
	"fmt"

	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/cli/tui/helpkeyer"
)

func main() {
	tview.Initialize()

	tv := tview.NewTextView()

	hk := helpkeyer.New()
	hk.Set('p', "pay", func() {
		fmt.Fprintln(tv, "Paid!")
	})
	hk.Set('f', "find", func() {
		fmt.Fprintln(tv, "Found!")
	})
	hk.Set('D', "This is so sad, Alexa play Despacito", func() {
		fmt.Fprintln(tv, "no")
	})
	hk.Set('e', "exit", func() {
		tview.Stop()
	})
	hk.Set('z', "z", func() {
		fmt.Fprintln(tv, "300")
	})
	hk.Set('x', "x", func() {
		fmt.Fprintln(tv, "300")
	})
	hk.Set('b', "watermelon", func() {
		fmt.Fprintln(tv, "Not found")
	})

	f := tview.NewFlex()
	f.SetDirection(tview.FlexRow)

	f.AddItem(tv, 0, 1, false)
	f.AddItem(hk, 1, 1, true)

	tview.SetRoot(f)

	if err := tview.Run(); err != nil {
		panic(err)
	}
}
