// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package log

import (
	"io"
	"sync"
)

type multiWriter struct {
	sync.RWMutex
	writers        map[string]io.Writer
	writersModules map[string]map[string]struct{}
}

func (t *multiWriter) SetWriter(key string, writer io.Writer, modules ...string) {
	t.Lock()
	defer t.Unlock()

	t.writers[key] = writer

	if len(modules) == 0 {
		t.writersModules[key] = nil
		return
	}

	// Make sure to clear the existing modules if the writer already exists.
	t.writersModules[key] = make(map[string]struct{})

	for i := range modules {
		t.writersModules[key][modules[i]] = struct{}{}
	}
}

func (t *multiWriter) Write(p []byte) (n int, err error) {
	t.RLock()
	defer t.RUnlock()

	for _, w := range t.writers {
		n, err = write(w, p)
		if err != nil {
			return
		}
	}
	return len(p), nil
}

// WriteFilter writes to only writers that have filter for the module.
func (t *multiWriter) WriteFilter(p []byte, module string) (n int, err error) {
	t.RLock()
	defer t.RUnlock()

	for k, w := range t.writers {
		if writerModules := t.writersModules[k]; writerModules != nil {
			if _, exist := writerModules[module]; !exist {
				continue
			}
		}

		n, err = write(w, p)
		if err != nil {
			return
		}
	}
	return len(p), nil
}

func (t *multiWriter) Clear() {
	t.Lock()
	defer t.Unlock()

	t.writers = make(map[string]io.Writer)
	t.writersModules = make(map[string]map[string]struct{})
}

func write(w io.Writer, p []byte) (n int, err error) {
	n, err = w.Write(p)
	if err != nil {
		return
	}

	if n != len(p) {
		err = io.ErrShortWrite
		return
	}

	return len(p), nil
}
