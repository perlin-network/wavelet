package logger

import (
	"sync"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
)

var _ tview.Primitive = (*Logger)(nil)

type Logger struct {
	list *tview.List
	evs  []*Event

	tv       *tview.TextView
	activeEv *Event // nil for list

	evMutex sync.Mutex

	focus bool
}

func NewLogger() *Logger {
	l := &Logger{}

	l.list = tview.NewList()
	l.list.SetMainTextColor(-1)
	l.list.SetSecondaryTextColor(-1)
	l.list.SetSelectedFunc(l.callEvent)
	l.list.SetHighlightFullLine(true)
	l.list.SetSelectedBackgroundColor(tcell.ColorGray)
	l.list.SetSelectedTextColor(tcell.ColorWhite)

	l.tv = tview.NewTextView()
	l.tv.SetDynamicColors(true)
	l.tv.SetTextColor(-1)
	l.tv.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		var key = event.Key()
		var char = event.Name()

		switch {
		case key == tcell.KeyEscape:
			if l.activeEv != nil {
				// Currently in the TextView buffer, gotta bail
				l.activeEv = nil
			}

		case key == tcell.KeyLeft, key == tcell.KeyRight,
			char == "h", char == "l":

			i := l.list.GetCurrentItem()
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
			l.activeEv = l.evs[i]

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
	l.activeEv = nil
}

func (l *Logger) Draw(s tcell.Screen) {
	if l.activeEv == nil {
		// Nothing selected, do active entry
		l.list.Draw(s)
	} else {
		// Something selected, draw the whole thing
		l.tv.SetText(l.activeEv.Full())
		l.tv.Draw(s)
	}
}

func (l *Logger) callEvent(i int, m, s string, r rune) {
	l.activeEv = l.evs[i]

	// Draw the application
	tview.Draw()
}

func (l *Logger) getPrimitive() tview.Primitive {
	if l.activeEv == nil {
		return l.list
	}

	return l.tv
}
