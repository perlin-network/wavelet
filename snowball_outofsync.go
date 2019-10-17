package wavelet

import (
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/sys"
	"sync"
)

type OutOfSyncSnowball struct {
	sync.RWMutex
	beta  int
	alpha int

	count  int
	counts map[bool]uint16

	preferred *outOfSyncVote
	last      *outOfSyncVote

	decided bool
}

func NewOutOfSyncSnowball() *OutOfSyncSnowball {
	return &OutOfSyncSnowball{
		counts: make(map[bool]uint16),
	}
}

func (s *OutOfSyncSnowball) Reset() {
	s.Lock()

	s.preferred = nil
	s.last = nil

	s.counts = make(map[bool]uint16)
	s.count = 0

	s.decided = false
	s.Unlock()
}

func (s *OutOfSyncSnowball) Tick(tallies map[bool]float64, votes map[bool]*outOfSyncVote) {
	s.Lock()
	defer s.Unlock()

	if s.decided {
		return
	}

	var majority *outOfSyncVote
	var majorityTally float64 = 0

	for id, tally := range tallies {
		if tally > majorityTally {
			majority, majorityTally = votes[id], tally
		}
	}

	denom := float64(len(votes))

	if denom < 2 {
		denom = 2
	}

	if majority == nil || majorityTally < conf.GetSnowballAlpha()*2/denom {
		s.count = 0
	} else {
		s.counts[majority.outOfSync] += 1

		if s.counts[majority.outOfSync] > s.counts[s.preferred.outOfSync] {
			s.preferred = majority
		}

		if s.last == nil || majority.outOfSync != s.last.outOfSync {
			s.last, s.count = majority, 1
		} else {
			s.count += 1

			if s.count > conf.GetSnowballBeta() {
				s.decided = true
			}
		}
	}
}

func (s *OutOfSyncSnowball) Prefer(o *outOfSyncVote) {
	s.Lock()
	s.preferred = o
	s.Unlock()
}

func (s *OutOfSyncSnowball) Preferred() *outOfSyncVote {
	s.RLock()
	preferred := s.preferred
	s.RUnlock()

	return preferred
}

func (s *OutOfSyncSnowball) Decided() bool {
	s.RLock()
	decided := s.decided
	s.RUnlock()

	return decided
}

func (s *OutOfSyncSnowball) Progress() int {
	s.RLock()
	progress := s.count
	s.RUnlock()

	return progress
}

func (s *OutOfSyncSnowball) ComputeStakeWeights(accounts *Accounts, responses []syncVote) map[bool]float64 {
	weights := make(map[bool]float64)

	var max float64

	snapshot := accounts.Snapshot()

	for _, res := range responses {
		stake, _ := ReadAccountStake(snapshot, res.voter.PublicKey())

		if stake < sys.MinimumStake {
			weights[res.outOfSync] += float64(sys.MinimumStake)
		} else {
			weights[res.outOfSync] += float64(stake)
		}

		if weights[res.outOfSync] > max {
			max = weights[res.outOfSync]
		}
	}

	for id := range weights {
		weights[id] /= max
	}

	return weights
}

func (s *OutOfSyncSnowball) Normalize(weights map[bool]float64) map[bool]float64 {
	normalized := make(map[bool]float64, len(weights))
	min, max := float64(1), float64(0)

	// Find minimum weight.
	for _, weight := range weights {
		if min > weight {
			min = weight
		}
	}

	// Subtract minimum and find maximum normalized weight.
	for vote, weight := range weights {
		normalized[vote] = weight - min

		if normalized[vote] > max {
			max = normalized[vote]
		}
	}

	// Normalize weight using maximum normalized weight into range [0, 1].
	for vote := range weights {
		if max == 0 {
			normalized[vote] = 1
		} else {
			normalized[vote] /= max
		}
	}

	return normalized
}
