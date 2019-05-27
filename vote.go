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

func CollectVotes(accounts *Accounts, snowball *Snowball, voteChan <-chan []vote, wg *sync.WaitGroup) {
COLLAPSE_VOTES:
	for votes := range voteChan {
		snapshot := accounts.Snapshot()

		voters := make(map[AccountID]struct{})

		for _, vote := range votes {
			if vote.voter == nil {
				continue COLLAPSE_VOTES
			}

			voters[vote.voter.PublicKey()] = struct{}{}
		}

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

		//fmt.Printf("%#v\n", weights)

		snowball.Tick(majority)

		voters = make(map[AccountID]struct{}, sys.SnowballK)
		votes = votes[:0]
	}

	if wg != nil {
		wg.Done()
	}
}
