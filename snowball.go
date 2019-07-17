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

package wavelet

import (
	"fmt"
	"sync"
)

type SnowballOption func(*Snowball)

func WithBeta(beta int) SnowballOption {
	return func(snowball *Snowball) {
		snowball.beta = beta
	}
}

func WithName(name string) SnowballOption {
	return func(snowball *Snowball) {
		snowball.name = name
	}
}

const (
	SnowballDefaultBeta = 150
)

type Identifiable interface {
	GetID() string
}

type Snowball struct {
	sync.RWMutex
	beta int

	candidates          map[string]Identifiable
	preferredID, lastID string

	counts  map[string]int
	count   int
	decided bool
	name string
}

func NewSnowball(opts ...SnowballOption) *Snowball {
	s := &Snowball{
		beta:       SnowballDefaultBeta,
		candidates: make(map[string]Identifiable),
		counts:     make(map[string]int),
		name: "default",
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *Snowball) Reset() {
	s.Lock()

	s.preferredID = ""
	s.lastID = ""

	s.candidates = make(map[string]Identifiable)
	s.counts = make(map[string]int)
	s.count = 0

	s.decided = false

	s.Unlock()
}

func (s *Snowball) Tick(v Identifiable) {
	s.Lock()
	defer s.Unlock()

	if s.decided { // Do not allow any further ticks until Reset() gets called.
		return
	}

	if v == nil || v.GetID() == "" { // Have nil responses reset Snowball.
		s.lastID = ""
		s.count = 0

		return
	}

	id := v.GetID()
	if _, exists := s.candidates[id]; !exists {
		s.candidates[id] = v
	}

	s.counts[id]++ // Handle decision case.

	if s.counts[id] > s.counts[s.preferredID] {
		s.preferredID = id
	}

	if s.lastID != id { // Handle termination case.
		if s.lastID != "" {
			fmt.Printf("Snowball (%s) liveness fault: Last ID is %x with count %d, and new ID is %x.\n", s.name, s.lastID, s.count, id)
		}

		s.lastID = id
		s.count = 0
	} else {
		s.count++

		if s.count > s.beta {
			s.decided = true
		}
	}
}

func (s *Snowball) Prefer(v Identifiable) {
	s.Lock()
	id := v.GetID()
	if _, exists := s.candidates[id]; !exists {
		s.candidates[id] = v
	}
	s.preferredID = id
	s.Unlock()
}

func (s *Snowball) Preferred() Identifiable {
	s.RLock()
	preferred := s.candidates[s.preferredID]
	s.RUnlock()

	return preferred
}

func (s *Snowball) Decided() bool {
	s.RLock()
	decided := s.decided
	s.RUnlock()

	return decided
}

func (s *Snowball) Progress() int {
	s.RLock()
	progress := s.count
	s.RUnlock()

	return progress
}
