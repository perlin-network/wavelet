package main

import (
	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/filechooser"
)

func main() {
	c := filechooser.New()
	c.FilePick = func(filename string) {
		tv := tview.NewTextView()
		tv.SetText(filename)

		tview.SetRoot(tv, true)
		tview.SetFocus(c)
	}

	if err := c.ChangeDirectory(""); err != nil {
		panic(err)
	}

	tview.Initialize()
	tview.SetRoot(c, true)
	tview.SetFocus(c)

	if err := tview.Run(); err != nil {
		panic(err)
	}
}

/*
func main() {
	tview.Initialize()
	go tview.Run()
	file := filechooser.Spawn()
	tview.Stop()
	fmt.Println(file)
}
*/
