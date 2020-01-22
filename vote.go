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
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"math"
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

// Return back the votes with their tallies calculated.
func calculateTallies(accounts *Accounts, responses []Vote) []Vote {
	votes := make(map[VoteID]Vote, len(responses))

	for _, res := range responses {
		if _, exists := votes[res.ID()]; !exists {
			votes[res.ID()] = res
		}

		votes[res.ID()].SetTally(votes[res.ID()].Tally() + 1.0/float64(len(responses)))
	}

	for id, weight := range Normalize(WeighByTransactions(responses)) {
		votes[id].SetTally(votes[id].Tally() * weight)
	}

	for id, weight := range Normalize(WeighByStake(accounts, responses)) {
		votes[id].SetTally(votes[id].Tally() * weight)
	}

	total := float64(0)
	for _, vote := range votes {
		total += vote.Tally()
	}

	tallies := make([]Vote, 0, len(votes))

	for _, vote := range votes {
		vote.SetTally(vote.Tally() / total)
		tallies = append(tallies, vote)
	}

	return tallies
}

func WeighByTransactions(responses []Vote) map[VoteID]float64 {
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

func WeighByStake(accounts *Accounts, responses []Vote) map[VoteID]float64 {
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
	min, max := math.MaxFloat64, math.SmallestNonzeroFloat64

	// Find minimum and maximum weights.
	for _, weight := range weights {
		if min > weight {
			min = weight
		}

		if max < weight {
			max = weight
		}
	}

	denom := max - min

	// Normalize weights into range [0, 1].
	for id := range weights {
		if denom <= 1e-30 {
			weights[id] = 1
		} else {
			weights[id] = (weights[id] - min) / denom
		}
	}

	return weights
}
