package conflict

import (
	"github.com/perlin-network/wavelet/common"
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

	preferred, last common.TransactionID

	counts map[common.TransactionID]int
	count  int

	decided bool
}

func NewSnowball() *snowball {
	return &snowball{
		k:     SnowballDefaultK,
		beta:  SnowballDefaultBeta,
		alpha: SnowballDefaultAlpha,

		counts: make(map[common.TransactionID]int),
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

	s.preferred = common.ZeroTransactionID
	s.last = common.ZeroTransactionID

	s.counts = make(map[common.TransactionID]int)
	s.count = 0

	s.decided = false
}

func (s *snowball) Tick(id common.TransactionID, votes []float64) {
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

func (s *snowball) Prefer(id common.TransactionID) {
	s.Lock()
	defer s.Unlock()

	s.preferred = id
}

func (s *snowball) Preferred() common.TransactionID {
	s.Lock()
	defer s.Unlock()

	return s.preferred
}

func (s *snowball) Decided() bool {
	s.Lock()
	defer s.Unlock()

	return s.decided
}
