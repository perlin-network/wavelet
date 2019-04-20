package wavelet

import (
	"github.com/perlin-network/wavelet/common"
)

const (
	SnowballDefaultK     = 1
	SnowballDefaultAlpha = float64(0.8)
	SnowballDefaultBeta  = 10
)

type Snowball struct {
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
	s.k = k
	return s
}

func (s *Snowball) WithAlpha(alpha float64) *Snowball {
	s.alpha = alpha
	return s
}

func (s *Snowball) WithBeta(beta int) *Snowball {
	s.beta = beta
	return s
}

func (s *Snowball) Reset() {
	s.preferredID = common.ZeroRoundID
	s.lastID = common.ZeroRoundID

	s.counts = make(map[common.RoundID]int)
	s.count = 0

	s.decided = false
}

func (s *Snowball) Tick(round *Round) {
	if s.decided { // Force Reset() to be manually called.
		return
	}

	if round == nil { // Do not let Snowball tick with nil responses.
		return
	}

	if _, exists := s.candidates[round.id]; !exists {
		s.candidates[round.id] = round
	}

	s.counts[round.id]++ // Handle decision case.

	if s.counts[round.id] > s.counts[s.preferredID] {
		s.preferredID = round.id
	}

	if s.lastID != round.id { // Handle termination case.
		s.lastID = round.id
		s.count = 0
	} else {
		s.count++

		if s.count > s.beta {
			s.decided = true
		}
	}
}

func (s *Snowball) Prefer(round *Round) {
	if _, exists := s.candidates[round.id]; !exists {
		s.candidates[round.id] = round
	}

	s.preferredID = round.id
}

func (s *Snowball) Preferred() *Round {
	if s.preferredID == common.ZeroRoundID {
		return nil
	}

	return s.candidates[s.preferredID]
}

func (s *Snowball) Decided() bool {
	return s.decided
}
