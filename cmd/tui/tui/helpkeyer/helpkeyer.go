// Package helpheyer provides a Primitive that controls keybinds while showing
// them, also provides an easy API to make keybinds.
package helpkeyer

import (
	"strings"
	"sync"

	"github.com/diamondburned/tcell"
	"github.com/diamondburned/tview/v2"
)

type HelpKeyer struct {
	*tview.TextView
	mu sync.Mutex

	// If false, run callbacks in goroutines
	Blocking bool

	Binds []*Bind

	inputCapture func(ev *tcell.EventKey) *tcell.EventKey
}

type Bind struct {
	Callback    func()
	Description string
	Bind        rune

	display string
	needle  int
}

func New() *HelpKeyer {
	h := &HelpKeyer{}

	h.TextView = tview.NewTextView()
	h.TextView.SetWordWrap(true)
	h.TextView.SetInputCapture(func(ev *tcell.EventKey) *tcell.EventKey {
		for _, b := range h.Binds {
			if b.Bind == ev.Rune() {
				if h.Blocking {
					b.Callback()
				} else {
					go b.Callback()
				}

				break
			}
		}

		if h.inputCapture != nil {
			return h.inputCapture(ev)
		}

		return nil
	})

	return h
}

// Set sets a bind into the help keyer
func (h *HelpKeyer) Set(bind rune, desc string, f func()) {
	b := &Bind{
		Callback:    f,
		Description: desc,
		Bind:        bind,
		// Needle set to -1 by default if it can't be found
		needle: -1,
	}

	// Try and find the needle
	for i, r := range desc {
		if r == bind {
			b.needle = i
			break
		}
	}

	if b.needle < 0 { // can't find needle
		// e.g. [r]to do
		b.display = "[" + string(bind) + "]" + desc
	} else {
		// e.g. to [d]o
		b.display = desc[:b.needle] + "[" + string(bind) + "]" + desc[b.needle+1:]
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	h.Binds = append(h.Binds, b)

	// Generate the list of keybinds for display
	var s strings.Builder
	for _, b := range h.Binds {
		s.WriteString(b.display + "\t")
	}

	h.SetText(s.String())
}

// SetInputCapture sets the function to execute if the rune binds cannot be
// found.
func (h *HelpKeyer) SetInputCapture(f func(*tcell.EventKey) *tcell.EventKey) {
	h.inputCapture = f
}
