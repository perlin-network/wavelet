package main

import (
	"github.com/atotto/clipboard"
	"github.com/fatih/color"
	"github.com/gdamore/tcell"
	"github.com/perlin-network/wavelet/log"
	"github.com/rivo/tview"
)

func main() {
	app := tview.NewApplication()
	defer app.Stop()

	app.SetBeforeDrawFunc(func(s tcell.Screen) bool {
		s.Clear()
		return false
	})

	header := tview.NewTextView()
	header.SetBackgroundColor(tcell.GetColor("#5f06fe"))
	header.SetTextColor(tcell.ColorWhite)
	header.SetDynamicColors(true)
	header.SetText(`⣿ wavelet v0.1.0`)

	header.Highlight("b")

	content := tview.NewTextView()
	content.SetWrap(true)
	content.SetWordWrap(true)
	content.SetBackgroundColor(tcell.ColorDefault)
	content.SetTextColor(tcell.ColorDefault)
	content.SetDynamicColors(true)
	content.SetBorderPadding(1, 1, 1, 1)
	content.SetChangedFunc(func() {
		app.ForceDraw()
	})
	content.SetScrollable(true)

	logger := log.Node()

	info := tview.NewTextView()
	info.SetText("⣿ Logged in as [yellow:black][696937c2c8df35dba0169de72990b80761e51dd9e2411fa1fce147f68ade830a][default:default]. You have 0 [#5cba3a]PERLs[default] available.")
	info.SetBackgroundColor(tcell.ColorDefault)
	info.SetTextColor(tcell.ColorDefault)
	info.SetDynamicColors(true)

	input := tview.NewInputField()
	input.SetFieldBackgroundColor(tcell.ColorDefault)
	input.SetFieldTextColor(tcell.ColorDefault)
	input.SetLabel(tview.TranslateANSI(color.New(color.Bold).Sprint("127.0.0.1:3000: ")))
	input.SetLabelColor(tcell.ColorDefault)
	input.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		switch event.Name() {
		case "Ctrl+V":
			txt, err := clipboard.ReadAll()
			if err != nil {
				return nil
			}

			input.SetText(input.GetText() + txt)

			return nil
		}

		return event
	})
	input.SetBackgroundColor(tcell.ColorDefault)
	input.SetPlaceholderTextColor(tcell.ColorDefault)
	input.SetPlaceholder("Enter a command...")
	input.SetBorderPadding(0, 0, 1, 0)
	input.SetBorderColor(tcell.ColorDefault)
	input.SetFieldWidth(0)
	input.SetDoneFunc(func(key tcell.Key) {
		switch key {
		case tcell.KeyEnter:
			cmd := input.GetText()

			if len(cmd) > 0 {
				content.Write([]byte("> " + cmd + "\n"))
				content.ScrollToEnd()

				input.SetText("")
			}
		}
	})
	input.SetBorder(true)

	log.SetWriter(log.LoggerWavelet, log.NewConsoleWriter(tview.ANSIWriter(content), log.FilterFor(log.ModuleNode)))

	logger.Info().Msg("Welcome to Wavelet.")
	logger.Info().Msg("Press " + color.New(color.Bold).Sprint("Enter") + " for a list of commands.")

	grid := tview.NewGrid().
		SetRows(1, 0, 1, 3).
		SetColumns(30, 0, 30)
	//SetBorders(true).
	//SetBordersColor(tcell.ColorDefault)

	grid.AddItem(header, 0, 0, 1, 30, 0, 0, false)
	grid.AddItem(content, 1, 0, 1, 30, 0, 0, false)

	grid.AddItem(info, 2, 0, 1, 30, 0, 0, false)
	grid.AddItem(input, 3, 0, 1, 30, 0, 0, true)

	app.SetRoot(grid, true)

	if err := app.Run(); err != nil {
		panic(err)
	}
}
