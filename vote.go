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

type Vote struct {
	id     VoteID
	voter  *skademlia.ID
	length float64
	value  interface{}
	tally  float64
}

func NewVoteFinalization(res finalizationVote) Vote {
	vote := Vote{
		id:     ZeroVoteID,
		voter:  res.voter,
		tally:  0,
		length: 0,
	}

	if res.block != nil {
		vote.value = res.block
		vote.id = res.block.ID
		vote.length = float64(len(res.block.Transactions))
	}

	return vote
}

func NewVoteSync(res syncVote) Vote {
	vote := Vote{
		id:     ZeroVoteID,
		voter:  res.voter,
		value:  res.outOfSync,
		tally:  0,
		length: 0,
	}

	// Use non-zero value to avoid conflict with ZeroVoteID.
	if res.outOfSync {
		binary.BigEndian.PutUint16(vote.id[:], 1)
	} else {
		binary.BigEndian.PutUint16(vote.id[:], 2)
	}

	return vote
}

func (v Vote) ID() VoteID {
	return v.id
}

func (v Vote) VoterID() AccountID {
	return v.voter.PublicKey()
}

func (v Vote) Length() float64 {
	return v.length
}

func (v Vote) Value() interface{} {
	return v.value
}

type syncVote struct {
	voter     *skademlia.ID
	outOfSync bool
}

type finalizationVote struct {
	voter *skademlia.ID
	block *Block
}

func CollectVotesForSync(
	accounts *Accounts,
	snowball *Snowball,
	voteChan <-chan syncVote,
	wg *sync.WaitGroup,
	snowballK int,
) {
	votes := make([]syncVote, 0, snowballK)
	voters := make(map[AccountID]struct{}, snowballK)

	snowball.Prefer(NewVoteSync(syncVote{
		outOfSync: false,
	}))

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

func TickForFinalization(accounts *Accounts, snowball *Snowball, responses []finalizationVote) {
	snowballResponses := make([]Vote, 0, len(responses))

	for i := range responses {
		snowballResponses = append(snowballResponses, NewVoteFinalization(responses[i]))
	}

	snowball.Tick(calculateTallies(accounts, snowballResponses))
}

func TickForSync(accounts *Accounts, snowball *Snowball, responses []syncVote) {
	snowballResponses := make([]Vote, 0, len(responses))

	for i := range responses {
		snowballResponses = append(snowballResponses, NewVoteSync(responses[i]))
	}

	snowball.Tick(calculateTallies(accounts, snowballResponses))
}

// Return back the votes with their tallies calculated.
func calculateTallies(accounts *Accounts, responses []Vote) []Vote {
	votes := make([]Vote, 0, len(responses))
	votesIndexByID := make(map[VoteID]int, len(responses))

	getVote := func(id VoteID) int {
		index, exist := votesIndexByID[id]
		if !exist {
			return -1
		}

		return index
	}

	for i := range responses {
		index := getVote(responses[i].ID())
		if index == -1 {
			index = len(votes)
			votes = append(votes, responses[i])

			votesIndexByID[responses[i].ID()] = index
		}

		votes[index].tally += 1.0 / float64(len(responses))
	}

	for id, weight := range Normalize(ComputeProfitWeights(responses)) {
		votes[getVote(id)].tally *= weight
	}

	for id, weight := range Normalize(ComputeStakeWeights(accounts, responses)) {
		votes[getVote(id)].tally *= weight
	}

	totalTally := float64(0)
	for _, block := range votes {
		totalTally += block.tally
	}

	for i := range votes {
		votes[i].tally /= totalTally
	}

	return votes
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
