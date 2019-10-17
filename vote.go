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

func CollectVotesForSync(
	accounts *Accounts,
	snowball *OutOfSyncSnowball,
	voteChan <-chan syncVote,
	wg *sync.WaitGroup,
	snowballK int,
) {
	votes := make([]syncVote, 0, snowballK)
	voters := make(map[AccountID]struct{}, snowballK)

	for vote := range voteChan {
		if _, recorded := voters[vote.voter.PublicKey()]; recorded {
			continue // To make sure the sampling process is fair, only allow one vote per peer.
		}

		voters[vote.voter.PublicKey()] = struct{}{}
		votes = append(votes, vote)

		if len(votes) == cap(votes) {

			//snapshot := accounts.Snapshot()

			tallies := make(map[bool]float64)
			blocks := make(map[bool]*outOfSyncVote)

			for _, vote := range votes {
				if _, exists := blocks[vote.outOfSync]; !exists {
					blocks[vote.outOfSync] = &outOfSyncVote{outOfSync: vote.outOfSync}
				}

				tallies[vote.outOfSync] += 1.0 / float64(len(votes))
			}

			stakeWeights := snowball.Normalize(snowball.ComputeStakeWeights(accounts, votes))
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

			voters = make(map[AccountID]struct{}, snowballK)
			votes = votes[:0]
		}
	}

	if wg != nil {
		wg.Done()
	}
}

func TickForFinalization(accounts *Accounts, snowball *BlockSnowball, votes []finalizationVote) {
	tallies := make(map[BlockID]float64)
	blocks := make(map[BlockID]*Block)

	for _, vote := range votes {
		if vote.block == nil {
			continue
		}

		if _, exists := blocks[vote.block.ID]; !exists {
			blocks[vote.block.ID] = vote.block
		}

		tallies[vote.block.ID] += 1.0 / float64(len(votes))
	}

	for block, weight := range snowball.Normalize(snowball.ComputeProfitWeights(votes)) {
		tallies[block] *= weight
	}

	stakeWeights := snowball.Normalize(snowball.ComputeStakeWeights(accounts, votes))
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
