package conflict

import (
	"sync"
)

const (
	SnowballDefaultK     = 1
	SnowballDefaultAlpha = float64(0.8)
	SnowballDefaultBeta  = 10
)

var _ Resolver = (*snowball)(nil)

type snowball struct {
	sync.RWMutex

	k, beta int
	alpha   float64

	preferred, last Item

	counts map[interface{}]int
	count  int

	decided bool
}

func NewSnowball() *snowball {
	return &snowball{
		k:     SnowballDefaultK,
		beta:  SnowballDefaultBeta,
		alpha: SnowballDefaultAlpha,

		counts: make(map[interface{}]int),
	}
}

func (s *snowball) WithK(k int) *snowball {
	s.Lock()
	s.k = k
	s.Unlock()

	return s
}

func (s *snowball) WithAlpha(alpha float64) *snowball {
	s.Lock()
	s.alpha = alpha
	s.Unlock()

	return s
}

func (s *snowball) WithBeta(beta int) *snowball {
	s.Lock()
	s.beta = beta
	s.Unlock()

	return s
}

func (s *snowball) Reset() {
	s.Lock()

	s.preferred = nil
	s.last = nil

	s.counts = make(map[interface{}]int)
	s.count = 0

	s.decided = false

	s.Unlock()
}

func (s *snowball) Tick(counts map[Item]float64) {
	s.Lock()
	defer s.Unlock()

	if s.decided {
		return
	}

	for preferred, count := range counts {
		if count >= s.alpha {
			s.counts[preferred.Hash()]++

			if s.preferred == nil || s.counts[preferred.Hash()] > s.counts[s.preferred.Hash()] {
				s.preferred = preferred
			}

			if s.last == nil || s.last.Hash() != preferred.Hash() {
				s.last = preferred
				s.count = 0
			} else {
				s.count++

				if s.count > s.beta {
					s.decided = true
				}
			}

			break
		}
	}
}

func (s *snowball) Prefer(id Item) {
	s.Lock()
	s.preferred = id
	s.Unlock()
}

func (s *snowball) Preferred() Item {
	s.RLock()
	preferred := s.preferred
	s.RUnlock()

	return preferred
}

func (s *snowball) Decided() bool {
	s.RLock()
	decided := s.decided
	s.RUnlock()

	return decided
}
