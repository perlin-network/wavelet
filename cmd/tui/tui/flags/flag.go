package flags

import (
	"fmt"
	"os"
	"strconv"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/spf13/pflag"
)

type Flag struct {
	tview.FormItem
}

// TODO: replace inputFields with a better inputFields that checks and prints
// errors properly

func NewFlag(flag *pflag.Flag) *Flag {
	f := Flag{}

	t := flag.Value.Type()

	if t == "bool" {
		i := tview.NewCheckbox()
		i.SetTitle(flag.Name)
		i.SetChangedFunc(func(b bool) {
			if err := flag.Value.Set(strconv.FormatBool(b)); err != nil {
				i.SetBackgroundColor(tcell.ColorRed)
			} else {
				i.SetBackgroundColor(tcell.ColorDefault)
			}
		})

		switch flag.Value.String() {
		case "true":
			i.SetChecked(true)
		case "false":
			i.SetChecked(false)
		}

		f.FormItem = i

	} else {
		i := tview.NewInputField()
		i.SetTitle(flag.Name)
		i.SetText(flag.Value.String())

		fmt.Fprintf(os.Stderr, "name=%s\tvalue=%s\n", flag.Name, flag.Value.String())

		i.SetChangedFunc(func(t string) {
			if t == "" {
				return
			}

			if err := flag.Value.Set(t); err != nil {
				i.SetBackgroundColor(tcell.ColorRed)
			} else {
				i.SetBackgroundColor(tcell.ColorDefault)
			}
		})

		switch t {
		case "int":
			i.SetAcceptanceFunc(tview.InputFieldInteger)
		case "float":
			i.SetAcceptanceFunc(tview.InputFieldFloat)
		}

		f.FormItem = i
	}

	return &f
}
