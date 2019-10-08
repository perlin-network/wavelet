package main

import (
	"fmt"
	"strconv"
	"unicode"

	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/forms"
)

func main() {
	var (
		BeatmapID int
		Title     string
	)

	f := forms.New()
	f.Height = 10

	bmIDPair := forms.NewPair("beatmap ID", func(output string) error {
		i, err := strconv.Atoi(output)
		if err != nil {
			return err
		}

		BeatmapID = i
		return nil
	})

	bmIDPair.Validator = forms.IntValidator()

	f.Add(bmIDPair)

	titlePair := forms.NewFromStringPtr("title", &Title)
	titlePair.Validator = forms.ORValidators(
		forms.RuneValidator(unicode.IsLetter),
		forms.RuneValidator(unicode.IsNumber),
	)

	f.Add(titlePair)

	tview.SetRoot(f, true)
	tview.SetFocus(f)

	go func() {
		if err := tview.Run(); err != nil {
			panic(err)
		}
	}()

	if !f.Spawn() {
		fmt.Println("Cancelled")
		return
	}

	tview.Stop()

	fmt.Println("BeatmapID", BeatmapID)
	fmt.Println("Title", Title)
}
