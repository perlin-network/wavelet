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
	"github.com/perlin-network/wavelet/conf"
	"sync"
)

type Snowball struct {
	sync.RWMutex

	count  int
	counts map[VoteID]uint16

	preferred Vote
	last      Vote

	decided bool
	stalled int
}

func NewSnowball() *Snowball {
	return &Snowball{
		counts: make(map[VoteID]uint16),
	}
}

func (s *Snowball) Reset() {
	s.Lock()

	s.counts = make(map[VoteID]uint16)
	s.count = 0

	s.decided = false
	s.stalled = 0

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
		// Empty vote can still have tally (the base tally), so we need to ignore empty vote.
		if vote.ID() == ZeroVoteID {
			continue
		}

		if majority == nil || vote.Tally() > majority.Tally() {
			majority = vote
		}
	}

	denom := float64(len(votes))

	if denom < 2 {
		denom = 2
	}

	if majority == nil || majority.Tally() < conf.GetSnowballAlpha()*2/denom {
		if s.preferred != nil {
			s.stalled++

			// TODO(kenta): configure stall
			if s.stalled > conf.GetSnowballBeta()*10 {
				s.preferred = nil
				s.stalled = 0
			}
		}

		s.count = 0

		return
	}

	s.counts[majority.ID()]++
	s.stalled = 0

	if s.preferred == nil || s.counts[majority.ID()] > s.counts[s.preferred.ID()] {
		s.preferred = majority
	}

	if s.last == nil || majority.ID() != s.last.ID() {
		s.last, s.count = majority, 1
	} else {
		s.count++

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
