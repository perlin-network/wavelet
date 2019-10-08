package flags

import (
	"strconv"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/urfave/cli"
)

type Int struct {
	*tview.InputField
	Noop

	Value int
	Flag  cli.IntFlag

	drawn bool
}

func NewInt(f cli.IntFlag) *Int {
	i := tview.NewInputField()
	i.SetLabel(f.Name)

	s := &Int{
		InputField: i,

		Noop: Noop{},
		Flag: f,
	}

	i.SetAcceptanceFunc(tview.InputFieldInteger)

	i.SetChangedFunc(func(str string) {
		s.Value, _ = strconv.Atoi(str)
	})

	i.SetDoneFunc(func(_ tcell.Key) {
		*s.Flag.Destination = s.Value
	})

	return s
}

func (s *Int) Draw(screen tcell.Screen) {
	if !s.drawn {
		s.SetText(strconv.Itoa(*s.Flag.Destination))
		s.drawn = true
	}

	s.InputField.Draw(screen)
}

func (s *Int) GetFlag() cli.Flag {
	return s.Flag
}
