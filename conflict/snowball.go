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
	sync.Mutex

	k, beta int
	alpha   float64

	preferred, last interface{}

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
	defer s.Unlock()

	s.k = k
	return s
}

func (s *snowball) WithAlpha(alpha float64) *snowball {
	s.Lock()
	defer s.Unlock()

	s.alpha = alpha
	return s
}

func (s *snowball) WithBeta(beta int) *snowball {
	s.Lock()
	defer s.Unlock()

	s.beta = beta
	return s
}

func (s *snowball) Reset() {
	s.Lock()
	defer s.Unlock()

	s.preferred = nil
	s.last = nil

	s.counts = make(map[interface{}]int)
	s.count = 0

	s.decided = false
}

func (s *snowball) Tick(counts map[interface{}]float64) {
	s.Lock()
	defer s.Unlock()

	for preferred, count := range counts {
		if count >= s.alpha {
			s.counts[preferred]++

			if s.counts[preferred] > s.counts[s.preferred] {
				s.preferred = preferred
			}

			if s.last != preferred {
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

func (s *snowball) Prefer(id interface{}) {
	s.Lock()
	defer s.Unlock()

	s.preferred = id
}

func (s *snowball) Preferred() interface{} {
	s.Lock()
	defer s.Unlock()

	return s.preferred
}

func (s *snowball) Decided() bool {
	s.Lock()
	defer s.Unlock()

	return s.decided
}
