package flags

import (
	"strconv"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/urfave/cli"
)

type Float struct {
	*tview.InputField
	Noop

	Value float64
	Flag  cli.Float64Flag

	drawn bool
}

func NewFloat(f cli.Float64Flag) *Float {
	i := tview.NewInputField()
	i.SetLabel(f.Name)

	s := &Float{
		InputField: i,

		Noop: Noop{},
		Flag: f,
	}

	i.SetAcceptanceFunc(tview.InputFieldFloat)

	i.SetChangedFunc(func(str string) {
		s.Value, _ = strconv.ParseFloat(str, 64)
	})

	i.SetDoneFunc(func(_ tcell.Key) {
		*s.Flag.Destination = s.Value
	})

	return s
}

func (s *Float) Draw(screen tcell.Screen) {
	if !s.drawn {
		s.SetText(strconv.FormatFloat(*s.Flag.Destination, 'f', -1, 64))
		s.drawn = true
	}

	s.InputField.Draw(screen)
}

func (s *Float) GetFlag() cli.Flag {
	return s.Flag
}
