package flags

import (
	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/urfave/cli"
)

type Bool struct {
	*tview.Checkbox
	Noop

	Value bool
	Flag  cli.BoolFlag

	drawn bool
}

func NewBool(f cli.BoolFlag) *Bool {
	c := tview.NewCheckbox()
	c.SetLabel(f.Name)

	s := &Bool{
		Checkbox: c,

		Noop: Noop{},
		Flag: f,
	}

	c.SetChangedFunc(func(checked bool) {
		s.Value = checked
	})

	c.SetDoneFunc(func(_ tcell.Key) {
		*s.Flag.Destination = s.Value
	})

	return s
}

func (s *Bool) Draw(screen tcell.Screen) {
	if !s.drawn {
		s.SetChecked(*s.Flag.Destination)
		s.drawn = true
	}

	s.Checkbox.Draw(screen)
}

func (s *Bool) GetFlag() cli.Flag {
	return s.Flag
}
