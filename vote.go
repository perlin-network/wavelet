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

type syncVote struct {
	voter     *skademlia.ID
	outOfSync bool
}

type finalizationVote struct {
	voter *skademlia.ID
	block *Block
}

// Represent a single vote for snowball.
type snowballVote struct {
	id    VoteID
	voter *skademlia.ID
	val   interface{}
	// Used to calculate profit weight, can be 0 if it's not applicable.
	length float64 // TODO find more suitable name ?
}

func newPreferredBlockVote(block *Block) *snowballVote {
	return &snowballVote{
		id:  block.ID,
		val: block,
	}
}

func newPreferredOutOfSyncVote(outOfSync bool) *snowballVote {
	return &snowballVote{
		id:  newBoolVoteID(outOfSync),
		val: &outOfSyncVote{outOfSync: outOfSync},
	}
}

type VoteID BlockID

func newBoolVoteID(b bool) VoteID {
	var voteID VoteID

	var id uint16
	if b {
		id = 1
	} else {
		id = 0
	}

	binary.BigEndian.PutUint16(voteID[:], id)

	return voteID
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

	// TODO is this the best place to set the initial preferred
	snowball.Prefer(newPreferredOutOfSyncVote(false))

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

func TickForFinalization(accounts *Accounts, snowball *Snowball, votes []finalizationVote) {
	snowballVotes := make([]*snowballVote, 0, len(votes))

	for _, vote := range votes {
		sv := &snowballVote{
			id:     vote.block.ID,
			voter:  vote.voter,
			length: float64(len(vote.block.Transactions)),
			val:    vote.block,
		}

		snowballVotes = append(snowballVotes, sv)
	}

	tick(accounts, snowball, snowballVotes)
}

func TickForSync(accounts *Accounts, snowball *Snowball, votes []syncVote) {
	snowballVotes := make([]*snowballVote, 0, len(votes))

	for _, vote := range votes {
		sv := &snowballVote{
			id:     newBoolVoteID(vote.outOfSync),
			voter:  vote.voter,
			length: 0,
			val:    &outOfSyncVote{outOfSync: vote.outOfSync},
		}

		snowballVotes = append(snowballVotes, sv)
	}

	tick(accounts, snowball, snowballVotes)
}

func tick(accounts *Accounts, snowball *Snowball, votes []*snowballVote) {
	tallies := make(map[VoteID]float64)
	blocks := make(map[VoteID]*snowballVote)

	for _, vote := range votes {
		if vote.val == nil {
			continue
		}

		if _, exists := blocks[vote.id]; !exists {
			blocks[vote.id] = vote
		}

		tallies[vote.id] += 1.0 / float64(len(votes))
	}

	for block, weight := range Normalize(ComputeProfitWeights(votes)) {
		tallies[block] *= weight
	}

	stakeWeights := Normalize(ComputeStakeWeights(accounts, votes))
	for block, weight := range stakeWeights {
		tallies[block] *= weight
	}

	totalTally := float64(0)
	for _, tally := range tallies {
		totalTally += tally
	}

	for block := range tallies {
		tallies[block] /= totalTally
	}

	snowball.Tick(tallies, blocks)
}

func ComputeProfitWeights(responses []*snowballVote) map[VoteID]float64 {
	weights := make(map[VoteID]float64)

	var max float64

	for _, res := range responses {
		if res.val == nil {
			continue
		}

		weights[res.id] += res.length

		if weights[res.id] > max {
			max = weights[res.id]
		}
	}

	for id := range weights {
		weights[id] /= max
	}

	return weights
}

func ComputeStakeWeights(accounts *Accounts, responses []*snowballVote) map[VoteID]float64 {
	weights := make(map[VoteID]float64)

	var max float64

	snapshot := accounts.Snapshot()

	for _, res := range responses {
		if res.val == nil {
			continue
		}

		stake, _ := ReadAccountStake(snapshot, res.voter.PublicKey())

		if stake < sys.MinimumStake {
			weights[res.id] += float64(sys.MinimumStake)
		} else {
			weights[res.id] += float64(stake)
		}

		if weights[res.id] > max {
			max = weights[res.id]
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
