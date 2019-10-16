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
	"sync"

	"github.com/perlin-network/wavelet/conf"
)

type SnowballOption func(*Snowball)

func WithName(name string) SnowballOption {
	return func(snowball *Snowball) {
		snowball.name = name
	}
}

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
	name    string
}

func NewSnowball(opts ...SnowballOption) *Snowball {
	s := &Snowball{
		candidates: make(map[string]Identifiable),
		counts:     make(map[string]int),
		name:       "default",
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

func (s *Snowball) Tick(tallies map[Identifiable]float64) {
	s.Lock()
	defer s.Unlock()

	if s.decided {
		return
	}

	var majority Identifiable = nil
	var majorityTally float64 = 0

	for id, tally := range tallies {
		if tally > majorityTally {
			majority, majorityTally = id, tally
		}
	}

	denom := float64(len(tallies))

	if denom < 2 {
		denom = 2
	}

	if majority == nil || majorityTally < conf.GetSnowballAlpha()*2/denom {
		s.count = 0
	} else {
		s.counts[majority.GetID()] += 1

		if s.counts[majority.GetID()] > s.counts[s.preferredID] {
			s.preferredID = majority.GetID()
		}

		if s.lastID == "" || majority.GetID() != s.lastID {
			s.lastID, s.count = majority.GetID(), 1
		} else {
			s.count += 1

			if s.count > conf.GetSnowballBeta() {
				s.decided = true
			}
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
