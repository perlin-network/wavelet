package log

import (
	"io"
	"sync"
)

type multiWriter struct {
	sync.RWMutex
	writers map[string]io.Writer
}

func (t *multiWriter) Set(key string, writer io.Writer) {
	t.Lock()
	defer t.Unlock()

	t.writers[key] = writer
}

func (t *multiWriter) Write(p []byte) (n int, err error) {
	t.RLock()
	defer t.RUnlock()

	for _, w := range t.writers {
		n, err = w.Write(p)
		if err != nil {
			return
		}
		if n != len(p) {
			err = io.ErrShortWrite
			return
		}
	}
	return len(p), nil
}
