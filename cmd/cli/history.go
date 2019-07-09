package main

/*
type History struct {
	Max   int
	Data  []string
	Index int

	mu sync.Mutex
}

func (h *History) Add(line string) {
	h.mu.Lock()
	defer h.mu.Unlock()

	prev := h.Prev()

	if line == prev {
		return
	}

	if len(h.Data) > h.Max {
		h.Data = append(h.Data[:0], h.Data[1:]...)
	}

	h.Data = append(h.Data, line)
	h.Index = len(h.Data) - 1
}

func (h *History) HasLine(line string) bool {
	for _, l := range h.Data {
		if line == l {
			return true
		}
	}

	return false
}

func (h *History) Get(index int) string {
	return h.Data[index]
}

func (h *History) Empty() bool {
	return len(h.Data) == 0
}

func (h *History) Prev() string {
	h.Index--

	if h.Index < 0 {
		h.Index = 0
	}

	if h.Empty() {
		return ""
	}

	return h.Data[h.Index]
}

func (h *History) Next() string {
	h.Index++

	if h.Index >= len(h.Data) {
		h.Index = len(h.Data) - 1
	}

	if h.Empty() {
		return ""
	}

	return h.Data[h.Index]
}

func (h *History) Update() {
	h.Index = len(h.Data)
}
*/
