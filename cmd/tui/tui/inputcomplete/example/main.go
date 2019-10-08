package main

import (
	"fmt"
	"strings"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/clearbg"
	"github.com/perlin-network/wavelet/cmd/tui/tui/inputcomplete"
)

var dict = []string{
	"uno", "dos", "tres", "cuatro", "cinco",
	"seis", "siete", "ocho", "nueve", "diez",
}

var eng = []string{
	"one", "two", "three", "four", "five",
	"six", "seven", "eight", "nine", "ten",
}

func main() {
	clearbg.Enable()

	i := inputcomplete.New()
	i.Completer = func(word string) []inputcomplete.Completion {
		// Make a slice of Completion entries
		cs := make([]inputcomplete.Completion, 0, len(dict))

		for i, d := range dict {
			// If the things in the dictionary starts with the input word
			if strings.HasPrefix(d, word) {
				// Add
				cs = append(cs, inputcomplete.Completion{
					// Color markup is supported
					Visual:  fmt.Sprintf("[white]%-7s [%s[][-]", d, eng[i]),
					Replace: d,
				})
			}
		}

		return cs
	}

	tv := tview.NewTextView()
	tv.SetBackgroundColor(tcell.ColorBlack)
	tv.SetText("Type the first 10 numbers in Spanish:\n" +
		strings.Join(dict, " "))

	f := tview.NewFlex()
	f.SetDirection(tview.FlexRow)
	f.AddItem(tv, 0, 1, false)
	f.AddItem(i, 1, 1, true)

	tview.Initialize()
	tview.SetRoot(f, true)
	tview.SetFocus(f)
	tview.Run()
}
