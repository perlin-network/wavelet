package flags

import (
	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/urfave/cli"
)

type String struct {
	*tview.InputField
	Noop

	Value string
	Flag  cli.StringFlag

	drawn bool
}

func NewString(f cli.StringFlag) *String {
	i := tview.NewInputField()
	i.SetLabel(f.Name)

	s := &String{
		InputField: i,

		Noop: Noop{},
		Flag: f,
	}

	i.SetChangedFunc(func(text string) {
		s.Value = text
	})

	i.SetDoneFunc(func(_ tcell.Key) {
		*s.Flag.Destination = s.Value
	})

	return s
}

func (s *String) Draw(screen tcell.Screen) {
	if !s.drawn {
		s.SetText(*s.Flag.Destination)
		s.drawn = true
	}

	s.InputField.Draw(screen)
}

func (s *String) GetFlag() cli.Flag {
	return s.Flag
}
