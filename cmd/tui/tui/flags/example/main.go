package main

import (
	"fmt"
	"os"

	"github.com/diamondburned/tview/v2"
	"github.com/perlin-network/wavelet/cmd/tui/tui/flags"
	"github.com/spf13/pflag"
)

var (
	Bool   bool
	Int    int
	Float  float64
	String string
)

func main() {
	fs := pflag.NewFlagSet("main", pflag.ExitOnError)

	fs.BoolVar(&Bool, "bool", true, "Test boolean")
	fs.IntVar(&Int, "int", 42, "Test int")
	fs.Float64Var(&Float, "float", 69.420, "Test float")
	fs.StringVar(&String, "string", "default string", "Test string")

	// Parse flags beforehand
	fs.Parse(os.Args)

	form := flags.New(fs)

	tview.Initialize()
	tview.SetRoot(form, true)
	tview.SetFocus(form)

	if err := tview.Run(); err != nil {
		panic(err)
	}

	fs.VisitAll(func(f *pflag.Flag) {
		fmt.Printf("name=%s\tvalue=%s\n", f.Name, f.Value.String())
	})
}
