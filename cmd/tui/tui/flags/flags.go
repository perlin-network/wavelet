package flags

import (
	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
	"github.com/urfave/cli"
)

type Flags struct {
	Fields []Field
}

type Field interface {
	GetFormField() tview.FormItem
	GetFlag() cli.Flag
}

type Noop struct {
	Changed func(string)
	Done    func(tcell.Key)
}

func (n *Noop) SetChangedFunc(f func(string)) {
	n.Changed = f
}
func (n *Noop) SetDoneFunc(f func(tcell.Key)) {
	n.Done = f
}
