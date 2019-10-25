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

type Snowball struct {
	sync.RWMutex
	beta  int
	alpha int

	count  int
	counts map[VoteID]uint16

	preferred Vote
	last      Vote

	decided bool
}

func NewSnowball() *Snowball {
	return &Snowball{
		counts: make(map[VoteID]uint16),
	}
}

func (s *Snowball) Reset() {
	s.Lock()

	s.preferred = nil
	s.last = nil

	s.counts = make(map[VoteID]uint16)
	s.count = 0

	s.decided = false
	s.Unlock()
}

func (s *Snowball) Tick(votes []Vote) {
	s.Lock()
	defer s.Unlock()

	if s.decided {
		return
	}

	var majority Vote

	for _, vote := range votes {
		if vote.ID() != ZeroVoteID && (majority == nil || vote.Tally() > majority.Tally()) {
			majority = vote
		}
	}

	denom := float64(len(votes))

	if denom < 2 {
		denom = 2
	}

	if majority == nil || majority.Tally() < conf.GetSnowballAlpha()*2/denom {
		s.count = 0
		return
	}

	s.counts[majority.ID()] += 1

	if s.preferred == nil || s.counts[majority.ID()] > s.counts[s.preferred.ID()] {
		s.preferred = majority
	}

	if s.last == nil || majority.ID() != s.last.ID() {
		s.last, s.count = majority, 1
	} else {
		s.count += 1

		if s.count > conf.GetSnowballBeta() {
			s.decided = true
		}
	}
}

func (s *Snowball) Prefer(b Vote) {
	s.Lock()
	s.preferred = b
	s.Unlock()
}

func (s *Snowball) Preferred() Vote {
	s.RLock()
	preferred := s.preferred
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
