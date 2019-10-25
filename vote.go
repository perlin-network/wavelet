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
	"encoding/binary"
	"github.com/perlin-network/wavelet/sys"
	"sync"

	"github.com/perlin-network/noise/skademlia"
)

type VoteID BlockID

var ZeroVoteID VoteID

type Vote interface {
	ID() VoteID
	VoterID() AccountID
	Length() float64
	Value() interface{}
	Tally() float64
	SetTally(v float64)
}

type syncVote struct {
	voter     *skademlia.ID
	outOfSync bool

	// This acts as cache since ID() can be called many times.
	// The value will be set when the ID() is called the first time.
	voteID VoteID
	tally  float64
}

func (s *syncVote) ID() VoteID {
	if s.voteID == ZeroVoteID {
		var voteID VoteID

		// Use non-zero value to avoid conflict with ZeroVoteID.
		var v uint16

		if s.outOfSync {
			v = 1
		} else {
			v = 2
		}

		binary.BigEndian.PutUint16(voteID[:], v)

		s.voteID = voteID
	}

	return s.voteID
}

func (s *syncVote) VoterID() AccountID {
	return s.voter.PublicKey()
}

func (s *syncVote) Length() float64 {
	// Not applicable, so we return 0
	return 0
}

func (s *syncVote) SetTally(v float64) {
	s.tally = v
}

func (s *syncVote) Tally() float64 {
	return s.tally
}

func (s *syncVote) Value() interface{} {
	return &s.outOfSync
}

type finalizationVote struct {
	voter *skademlia.ID
	block *Block

	tally float64
}

// If the block is empty, it will return ZeroVoteID.
func (f *finalizationVote) ID() VoteID {
	if f.block == nil {
		return ZeroVoteID
	}
	return f.block.ID
}

func (f *finalizationVote) VoterID() AccountID {
	return f.voter.PublicKey()
}

// If the block is empty, it will return 0.
func (f *finalizationVote) Length() float64 {
	if f.block == nil {
		return 0
	}
	return float64(len(f.block.Transactions))
}

// If the block is empty, it will return nil.
func (f *finalizationVote) Value() interface{} {
	return f.block
}

func (f *finalizationVote) SetTally(v float64) {
	f.tally = v
}

func (f *finalizationVote) Tally() float64 {
	return f.tally
}

func CollectVotesForSync(
	accounts *Accounts,
	snowball *Snowball,
	voteChan <-chan *syncVote,
	wg *sync.WaitGroup,
	snowballK int,
) {
	votes := make([]*syncVote, 0, snowballK)
	voters := make(map[AccountID]struct{}, snowballK)

	// TODO is this the best place to set the initial preferred
	snowball.Prefer(&syncVote{
		outOfSync: false,
	})

	for vote := range voteChan {
		if _, recorded := voters[vote.voter.PublicKey()]; recorded {
			continue // To make sure the sampling process is fair, only allow one vote per peer.
		}

		voters[vote.voter.PublicKey()] = struct{}{}
		votes = append(votes, vote)

		if len(votes) == cap(votes) {
			TickForSync(accounts, snowball, votes)

			voters = make(map[AccountID]struct{}, snowballK)
			votes = votes[:0]
		}
	}

	if wg != nil {
		wg.Done()
	}
}

func TickForFinalization(accounts *Accounts, snowball *Snowball, responses []*finalizationVote) {
	snowballResponses := make([]Vote, 0, len(responses))

	for _, res := range responses {
		snowballResponses = append(snowballResponses, res)
	}

	tick(accounts, snowball, snowballResponses)
}

func TickForSync(accounts *Accounts, snowball *Snowball, responses []*syncVote) {
	snowballResponses := make([]Vote, 0, len(responses))

	for _, res := range responses {
		snowballResponses = append(snowballResponses, res)
	}

	tick(accounts, snowball, snowballResponses)
}

func tick(accounts *Accounts, snowball *Snowball, responses []Vote) {
	votes := make(map[VoteID]Vote, len(responses))

	for _, res := range responses {
		if _, exists := votes[res.ID()]; !exists {
			votes[res.ID()] = res
		}

		res.SetTally(res.Tally() + 1.0/float64(len(responses)))
	}

	for id, weight := range Normalize(ComputeProfitWeights(responses)) {
		votes[id].SetTally(votes[id].Tally() * weight)
	}

	stakeWeights := Normalize(ComputeStakeWeights(accounts, responses))
	for id, weight := range stakeWeights {
		votes[id].SetTally(votes[id].Tally() * weight)
	}

	totalTally := float64(0)
	for _, block := range votes {
		totalTally += block.Tally()
	}

	// Put the votes into slice to pass to snowball.
	array := make([]Vote, 0, len(votes))
	for id := range votes {
		votes[id].SetTally(votes[id].Tally() / totalTally)

		array = append(array, votes[id])
	}

	snowball.Tick(array)
}

func ComputeProfitWeights(responses []Vote) map[VoteID]float64 {
	weights := make(map[VoteID]float64, len(responses))

	var max float64

	for _, res := range responses {
		if res.ID() == ZeroVoteID {
			continue
		}

		weights[res.ID()] += res.Length()

		if weights[res.ID()] > max {
			max = weights[res.ID()]
		}
	}

	for id := range weights {
		weights[id] /= max
	}

	return weights
}

func ComputeStakeWeights(accounts *Accounts, responses []Vote) map[VoteID]float64 {
	weights := make(map[VoteID]float64, len(responses))

	var max float64

	snapshot := accounts.Snapshot()

	for _, res := range responses {
		if res.ID() == ZeroVoteID {
			continue
		}

		stake, _ := ReadAccountStake(snapshot, res.VoterID())

		if stake < sys.MinimumStake {
			weights[res.ID()] += float64(sys.MinimumStake)
		} else {
			weights[res.ID()] += float64(stake)
		}

		if weights[res.ID()] > max {
			max = weights[res.ID()]
		}
	}

	for id := range weights {
		weights[id] /= max
	}

	return weights
}

func Normalize(weights map[VoteID]float64) map[VoteID]float64 {
	normalized := make(map[VoteID]float64, len(weights))
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
