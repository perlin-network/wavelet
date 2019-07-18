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

type Snowball struct {
	sync.RWMutex
	beta int

	candidates          map[RoundID]*Round
	preferredID, lastID RoundID

	counts  map[RoundID]int
	count   int
	decided bool
	name    string
}

func NewSnowball(opts ...SnowballOption) *Snowball {
	s := &Snowball{
		beta:       SnowballDefaultBeta,
		candidates: make(map[RoundID]*Round),
		counts:     make(map[RoundID]int),
		name:       "default",
	}

	for _, opt := range opts {
		opt(s)
	}

	return s
}

func (s *Snowball) Reset() {
	s.Lock()

	s.preferredID = ZeroRoundID
	s.lastID = ZeroRoundID

	s.candidates = make(map[RoundID]*Round)
	s.counts = make(map[RoundID]int)
	s.count = 0

	s.decided = false

	s.Unlock()
}

func (s *Snowball) Tick(round *Round) {
	s.Lock()
	defer s.Unlock()

	if s.decided { // Do not allow any further ticks until Reset() gets called.
		return
	}

	if round == nil || round.ID == ZeroRoundID { // Have nil responses reset Snowball.
		s.lastID = ZeroRoundID
		s.count = 0

		return
	}

	if _, exists := s.candidates[round.ID]; !exists {
		s.candidates[round.ID] = round
	}

	s.counts[round.ID]++ // Handle decision case.

	if s.counts[round.ID] > s.counts[s.preferredID] {
		s.preferredID = round.ID
	}

	if s.lastID != round.ID { // Handle termination case.
		if s.lastID != ZeroRoundID {
			fmt.Printf("Snowball (%s) liveness fault: Last ID is %x with count %d, and new ID is %x.\n", s.name, s.lastID, s.count, round.ID)
		}

		s.lastID = round.ID
		s.count = 0
	} else {
		s.count++

		if s.count > s.beta {
			s.decided = true
		}
	}
}

func (s *Snowball) Prefer(round *Round) {
	s.Lock()
	if _, exists := s.candidates[round.ID]; !exists {
		s.candidates[round.ID] = round
	}
	s.preferredID = round.ID
	s.Unlock()
}

func (s *Snowball) Preferred() *Round {
	s.RLock()
	if s.preferredID == ZeroRoundID {
		s.RUnlock()
		return nil
	}
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
