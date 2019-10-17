package wavelet

import (
	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/sys"
	"golang.org/x/crypto/blake2b"
	"sync"
)

type BlockSnowball struct {
	sync.RWMutex
	beta  int
	alpha int

	count  int
	counts map[[blake2b.Size256]byte]uint16

	preferred *Block
	last      *Block

	decided bool
}

func NewBlockSnowball() *BlockSnowball {
	return &BlockSnowball{
		counts: make(map[[blake2b.Size256]byte]uint16),
	}
}

func (s *BlockSnowball) Reset() {
	s.Lock()

	s.preferred = nil
	s.last = nil

	s.counts = make(map[[blake2b.Size256]byte]uint16)
	s.count = 0

	s.decided = false
	s.Unlock()
}

func (s *BlockSnowball) Tick(tallies map[[blake2b.Size256]byte]float64, votes map[[blake2b.Size256]byte]*Block) {
	s.Lock()
	defer s.Unlock()

	if s.decided {
		return
	}

	var majority *Block
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
		s.counts[majority.ID] += 1

		if s.counts[majority.ID] > s.counts[s.preferred.ID] {
			s.preferred = majority
		}

		if s.last == nil || majority.ID != s.last.ID {
			s.last, s.count = majority, 1
		} else {
			s.count += 1

			if s.count > conf.GetSnowballBeta() {
				s.decided = true
			}
		}
	}
}

func (s *BlockSnowball) Prefer(b *Block) {
	s.Lock()
	s.preferred = b
	s.Unlock()
}

func (s *BlockSnowball) Preferred() *Block {
	s.RLock()
	preferred := s.preferred
	s.RUnlock()

	return preferred
}

func (s *BlockSnowball) Decided() bool {
	s.RLock()
	decided := s.decided
	s.RUnlock()

	return decided
}

func (s *BlockSnowball) Progress() int {
	s.RLock()
	progress := s.count
	s.RUnlock()

	return progress
}

func (s *BlockSnowball) ComputeProfitWeights(responses []finalizationVote) map[BlockID]float64 {
	weights := make(map[BlockID]float64)

	var max float64

	for _, res := range responses {
		if res.block == nil {
			continue
		}

		weights[res.block.ID] += float64(len(res.block.Transactions))

		if weights[res.block.ID] > max {
			max = weights[res.block.ID]
		}
	}

	for id := range weights {
		weights[id] /= max
	}

	return weights
}

func (s *BlockSnowball) ComputeStakeWeights(accounts *Accounts, responses []finalizationVote) map[BlockID]float64 {
	weights := make(map[BlockID]float64)

	var max float64

	snapshot := accounts.Snapshot()

	for _, res := range responses {
		if res.block == nil {
			continue
		}

		stake, _ := ReadAccountStake(snapshot, res.voter.PublicKey())

		if stake < sys.MinimumStake {
			weights[res.block.ID] += float64(sys.MinimumStake)
		} else {
			weights[res.block.ID] += float64(stake)
		}

		if weights[res.block.ID] > max {
			max = weights[res.block.ID]
		}
	}

	for id := range weights {
		weights[id] /= max
	}

	return weights
}

func (s *BlockSnowball) Normalize(weights map[BlockID]float64) map[BlockID]float64 {
	normalized := make(map[BlockID]float64, len(weights))
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
