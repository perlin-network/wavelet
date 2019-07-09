package logger

import (
	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
)

var _ tview.Primitive = (*Logger)(nil)

type Logger struct {
	list *tview.List
	lvls []Level

	tv        *tview.TextView
	activeLvl Level // nil for list

	focus bool
}

func NewLogger() *Logger {
	l := &Logger{}

	l.list = tview.NewList()
	l.list.SetSecondaryTextColor(tcell.ColorDefault)
	l.list.SetSelectedFunc(l.callLevel)
	l.list.SetHighlightFullLine(true)

	l.tv = tview.NewTextView()
	l.tv.SetDynamicColors(true)
	l.tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		var key = event.Key()
		var char = event.Name()

		switch {
		case key == tcell.KeyEscape:
			if l.activeLvl != nil {
				// Currently in the TextView buffer, gotta bail
				l.activeLvl = nil
			}

		case key == tcell.KeyLeft, key == tcell.KeyRight,
			char == "h", char == "l":

			i, _ := l.list.GetCurrentItem()
			c := l.list.GetItemCount()

			// Left/H to scroll back, right/L to scroll forth
			if key == tcell.KeyLeft || char == "h" {
				i--
			} else {
				i++
			}

			// current item after moving is out of range, bail
			if i < 0 || i >= c {
				return nil
			}

			l.list.SetCurrentItem(i)
			l.activeLvl = l.lvls[i]

			return nil
		}

		return event
	})

	return l
}

func (l *Logger) SetRect(x, y, w, h int) {
	l.list.SetRect(x, y, w, h)
	l.tv.SetRect(x, y, w, h)
}

func (l *Logger) GetRect() (int, int, int, int) {
	return l.list.GetRect()
}

func (l *Logger) InputHandler() func(*tcell.EventKey, func(tview.Primitive)) {
	return l.getPrimitive().InputHandler()
}

func (l *Logger) Focus(delegate func(tview.Primitive)) {
	l.focus = true
}

func (l *Logger) HasFocus() bool {
	return l.focus
}

func (l *Logger) GetFocusable() tview.Focusable {
	return l
}

func (l *Logger) Blur() {
	l.activeLvl = nil
}

func (l *Logger) Draw(s tcell.Screen) {
	if l.activeLvl == nil {
		// Nothing selected, do active entry
		l.list.Draw(s)
	} else {
		// Something selected, draw the whole thing
		l.tv.SetText(l.activeLvl.Full())
		l.tv.Draw(s)
	}
}

func (l *Logger) Level(lvl Level) *Logger {
	// Add the level to the state list
	l.lvls = append(l.lvls, lvl)

	// Add the level to the list as well
	m, s := lvl.Short()
	l.list.AddItem(m, "          "+s, 0, nil)

	i, _ := l.list.GetCurrentItem()
	c := l.list.GetItemCount()

	if i > c-3 && l.activeLvl == nil || !l.focus {
		// The Logger is not focused, or the cursor is around the list and we're
		// still on the list, just scroll to bottom.
		l.list.SetCurrentItem(c - 1)
	}

	// Draw the application
	tview.ExecApplication(func(app *tview.Application) bool {
		return true
	})

	return l
}

func (l *Logger) callLevel(i int, m, s string, r rune) {
	l.activeLvl = l.lvls[i]

	// Draw the application
	tview.ExecApplication(func(app *tview.Application) bool {
		return true
	})
}

func (l *Logger) getPrimitive() tview.Primitive {
	if l.activeLvl == nil {
		return l.list
	}

	return l.tv
}
