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
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"sync"
)

type vote struct {
	voter *skademlia.ID
	value Identifiable
}

func CollectVotes(accounts *Accounts, snowball *Snowball, voteChan <-chan vote, wg *sync.WaitGroup, snowballK int) {
	votes := make([]vote, 0, snowballK)
	voters := make(map[AccountID]struct{}, snowballK)

	for vote := range voteChan {
		if _, recorded := voters[vote.voter.PublicKey()]; recorded {
			continue // To make sure the sampling process is fair, only allow one vote per peer.
		}

		voters[vote.voter.PublicKey()] = struct{}{}
		votes = append(votes, vote)

		if len(votes) == cap(votes) {
			snapshot := accounts.Snapshot()

			stakes := make(map[AccountID]float64, len(votes))
			maxStake := float64(0)

			for _, vote := range votes {
				s, _ := ReadAccountStake(snapshot, vote.voter.PublicKey())

				if s < sys.MinimumStake {
					s = sys.MinimumStake
				}

				stake := float64(s)
				stakes[vote.voter.PublicKey()] = stake

				if maxStake < stake {
					maxStake = stake
				}
			}

			counts := make(map[string]float64, len(votes))
			totalCount := float64(0)

			for _, vote := range votes {
				count := stakes[vote.voter.PublicKey()] / maxStake
				counts[vote.value.GetID()] += count
				totalCount += count
			}

			var majority Identifiable
			for _, vote := range votes {
				if counts[vote.value.GetID()]/totalCount >= sys.SnowballAlpha {
					majority = vote.value
					break
				}
			}

			snowball.Tick(majority)

			voters = make(map[AccountID]struct{}, snowballK)
			votes = votes[:0]
		}
	}

	if wg != nil {
		wg.Done()
	}
}
