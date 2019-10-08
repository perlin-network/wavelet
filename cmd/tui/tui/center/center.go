package center

import "github.com/diamondburned/tview/v2"

type Center struct {
	MaxWidth  int
	MaxHeight int

	x, y int
	w, h int

	tview.Primitive
}

var _ tview.Primitive = (*Center)(nil)

func New(p tview.Primitive) *Center {
	return &Center{
		Primitive: p,
	}
}

// GetRect overrides the embedded Primitive's GetRect method.
func (c *Center) GetRect() (int, int, int, int) {
	return c.x, c.y, c.w, c.h
}

// SetRect overrides the embedded Primitive's SetRect method.
func (c *Center) SetRect(x, y, w, h int) {
	c.x, c.y = x, y
	c.w, c.h = w, h

	// Get the default primitive positions and sizes
	var (
		pW, pH = w, h
		pX, pY = x, y
	)

	// If the height is bigger than the max height
	if h > c.MaxHeight {
		pH = c.MaxHeight           // min
		pY = y + (h-c.MaxHeight)/2 // also center the primitive
	}

	// If the width is bigger than the max width
	if w > c.MaxWidth {
		pW = c.MaxWidth           // min
		pX = x + (w-c.MaxWidth)/2 // center
	}

	c.Primitive.SetRect(pX, pY, pW, pH)
}
