package wavelet

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"sync"
)

type vote struct {
	voter     *skademlia.ID
	preferred *Round
}

func CollectVotes(accounts *Accounts, snowball *Snowball, voteChan <-chan vote, wg *sync.WaitGroup) {
	votes := make([]vote, 0, sys.SnowballK)
	voters := make(map[AccountID]struct{}, sys.SnowballK)

	for vote := range voteChan {
		if _, recorded := voters[vote.voter.PublicKey()]; recorded {
			continue // To make sure the sampling process is fair, only allow one vote per peer.
		}

		voters[vote.voter.PublicKey()] = struct{}{}
		votes = append(votes, vote)

		if len(votes) == cap(votes) {
			snapshot := accounts.Snapshot()

			weights := computeStakeDistribution(snapshot, voters)
			counts := make(map[RoundID]float64, len(votes))

			var majority *Round

			for _, vote := range votes {
				if vote.preferred == nil {
					vote.preferred = ZeroRoundPtr
				}

				counts[vote.preferred.ID] += weights[vote.voter.PublicKey()]

				if counts[vote.preferred.ID] >= sys.SnowballAlpha {
					majority = vote.preferred
					break
				}
			}

			snowball.Tick(majority)

			voters = make(map[AccountID]struct{}, sys.SnowballK)
			votes = votes[:0]
		}
	}

	if wg != nil {
		wg.Done()
	}
}
