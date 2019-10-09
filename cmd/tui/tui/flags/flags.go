package flags

import (
	"github.com/diamondburned/tview/v2"
	"github.com/spf13/pflag"
)

type Flags struct {
	*tview.Form
	Flags []*Flag
}

func New(fs *pflag.FlagSet) *Flags {
	flags := Flags{}
	form := tview.NewForm()

	fs.VisitAll(func(f *pflag.Flag) {
		flag := NewFlag(f)
		form.AddFormItem(flag)

		flags.Flags = append(flags.Flags, NewFlag(f))
	})

	form.AddButton("Ok", func() {
		tview.Stop()
	})

	flags.Form = form
	return &flags
}
