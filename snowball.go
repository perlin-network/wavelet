package wavelet

import (
	"github.com/perlin-network/wavelet/common"
	"sync"
)

const (
	SnowballDefaultK     = 1
	SnowballDefaultAlpha = float64(0.8)
	SnowballDefaultBeta  = 10
)

type Snowball struct {
	sync.RWMutex

	k, beta int
	alpha   float64

	candidates          map[common.RoundID]*Round
	preferredID, lastID common.RoundID

	counts map[common.RoundID]int
	count  int

	decided bool
}

func NewSnowball() *Snowball {
	return &Snowball{
		k:     SnowballDefaultK,
		beta:  SnowballDefaultBeta,
		alpha: SnowballDefaultAlpha,

		counts:     make(map[common.RoundID]int),
		candidates: make(map[common.RoundID]*Round),
	}
}

func (s *Snowball) WithK(k int) *Snowball {
	s.Lock()
	defer s.Unlock()

	s.k = k
	return s
}

func (s *Snowball) WithAlpha(alpha float64) *Snowball {
	s.Lock()
	defer s.Unlock()

	s.alpha = alpha
	return s
}

func (s *Snowball) WithBeta(beta int) *Snowball {
	s.Lock()
	defer s.Unlock()

	s.beta = beta
	return s
}

func (s *Snowball) Reset() {
	s.Lock()
	defer s.Unlock()

	s.preferredID = common.ZeroRoundID
	s.lastID = common.ZeroRoundID

	s.counts = make(map[common.RoundID]int)
	s.count = 0

	s.decided = false
}

func (s *Snowball) Tick(round *Round) {
	if round == nil { // Do not let Snowball tick with nil responses.
		return
	}

	s.Lock()
	defer s.Unlock()

	if s.decided { // Force Reset() to be manually called.
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
	defer s.Unlock()

	if _, exists := s.candidates[round.ID]; !exists {
		s.candidates[round.ID] = round
	}

	s.preferredID = round.ID
}

func (s *Snowball) Preferred() *Round {
	s.RLock()
	defer s.RUnlock()

	if s.preferredID == common.ZeroRoundID {
		return nil
	}

	return s.candidates[s.preferredID]
}

func (s *Snowball) Decided() bool {
	s.RLock()
	defer s.RUnlock()

	return s.decided
}
