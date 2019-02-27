package conflict

import (
	"golang.org/x/crypto/blake2b"
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

	preferred, last [blake2b.Size256]byte

	counts map[[blake2b.Size256]byte]int
	count  int

	decided bool
}

func NewSnowball() *snowball {
	return &snowball{
		k:     SnowballDefaultK,
		beta:  SnowballDefaultBeta,
		alpha: SnowballDefaultAlpha,

		counts: make(map[[blake2b.Size256]byte]int),
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

	s.preferred = [blake2b.Size256]byte{}
	s.last = [blake2b.Size256]byte{}

	s.counts = make(map[[blake2b.Size256]byte]int)
	s.count = 0

	s.decided = false
}

func (s *snowball) Tick(id [blake2b.Size256]byte, votes []float64) {
	s.Lock()
	defer s.Unlock()

	var tally float64

	for _, vote := range votes {
		tally += vote

		if tally >= s.alpha {
			s.counts[id]++

			if s.counts[id] > s.counts[s.preferred] {
				s.preferred = id
			}

			if s.last != id {
				s.last = id
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

func (s *snowball) Prefer(id [blake2b.Size256]byte) {
	s.Lock()
	defer s.Unlock()

	s.preferred = id
}

func (s *snowball) Preferred() [blake2b.Size256]byte {
	s.Lock()
	defer s.Unlock()

	return s.preferred
}

func (s *snowball) Decided() bool {
	s.Lock()
	defer s.Unlock()

	return s.decided
}
