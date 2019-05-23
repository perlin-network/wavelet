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
}

func NewSnowball(opts ...SnowballOption) *Snowball {
	s := &Snowball{
		beta:       SnowballDefaultBeta,
		candidates: make(map[RoundID]*Round),
		counts:     make(map[RoundID]int),
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
	if round == nil || round.ID == ZeroRoundID { // Do not let Snowball tick with nil responses.
		return
	}

	s.Lock()
	defer s.Unlock()

	if s.decided { // Do not allow any further ticks until Reset() gets called.
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
			fmt.Printf("Snowball liveness fault: Last ID is %x with count %d, and new ID is %x.\n", s.lastID, s.count, round.ID)
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
