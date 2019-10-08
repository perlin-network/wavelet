package inputcomplete

import (
	"strings"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
)

func isDown(ev *tcell.EventKey) bool {
	if ev.Modifiers() == tcell.ModCtrl && ev.Rune() == 'j' {
		return true
	}

	return ev.Key() == tcell.KeyDown
}

func isUp(ev *tcell.EventKey) bool {
	if ev.Modifiers() == tcell.ModCtrl && ev.Rune() == 'k' {
		return true
	}

	return ev.Key() == tcell.KeyUp
}

func (i *Input) inputBinds(ev *tcell.EventKey) *tcell.EventKey {
	defer tview.Draw()

	switch {
	// TODO: Instead of focusing, try handling keys into the Complete List
	// while still keeping focus onto the InputField
	case isDown(ev), isUp(ev):
		it := i.Complete.GetCurrentItem()

		switch {
		case isUp(ev):
			it--
			if it < 0 {
				it = i.Complete.GetItemCount() - 1
			}
		case isDown(ev):
			it++
			if it > i.Complete.GetItemCount()-1 {
				it = 0
			}
		}

		i.Complete.SetCurrentItem(it)
		return nil

	case ev.Key() == tcell.KeyEnter:
		// TODO: Investigate why the Mutex freezes the app
		//i.completeMutex.Lock()
		//defer i.completeMutex.Unlock()

		if len(i.completes) > 0 {
			// Get "to", the item to be completed
			item := i.Complete.GetCurrentItem()
			to := i.Complete.GetItem(item).SecondaryText

			// Get the current text
			text := i.InputField.GetText()

			// Append a space if needed
			if i.CompletionSpace {
				to += " "
			}

			/* Overthinking lol
			pos := i.InputField.CursorPos

			var from = 0
			for i := pos - 1; i > 0; i-- {
				if text[i] == ' ' {
					from = i + 1
					break
				}
			}

			text = text[:from] + to + text[pos:]
			i.InputField.SetText(text)
			*/

			// Get the text without the replaced part
			f := strings.Split(text, " ")
			text = strings.Join(f[:len(f)-1], " ")
			if len(f) > 1 {
				text += " "
			}

			// Set the text and set the focus
			i.InputField.SetText(text + to)

			// Clear the list after selection. Mutex setter is not used, as
			// mutex is already locked above.
			i.completes = nil

			// Prevent jumping to the next field.
			return nil
		} else {
			// return ev, Enter confirms the field and jumps to the next one.
			return ev
		}
	}

	return ev
}
