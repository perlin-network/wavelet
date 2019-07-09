package main

import "github.com/diamondburned/tview/v2"

func runTUI() error {
	tview.Initialize()

	return tview.Run()
}
