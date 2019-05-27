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

			counts := make(map[RoundID]float64, len(votes))
			stakes := make(map[RoundID]float64, len(votes))

			for _, vote := range votes {
				if vote.preferred == nil {
					vote.preferred = ZeroRoundPtr
				}

				counts[vote.preferred.ID] += 1.0

				stake, _ := ReadAccountStake(snapshot, vote.voter.PublicKey())

				if stake < sys.MinimumStake {
					stake = sys.MinimumStake
				}

				stakes[vote.preferred.ID] += float64(stake)
			}

			maxCount := float64(0)
			maxStake := float64(0)

			for _, vote := range votes {
				if vote.preferred == nil {
					vote.preferred = ZeroRoundPtr
				}

				if maxCount < counts[vote.preferred.ID] {
					maxCount = counts[vote.preferred.ID]
				}

				if maxStake < stakes[vote.preferred.ID] {
					maxStake = stakes[vote.voter.PublicKey()]
				}
			}

			for _, vote := range votes {
				if vote.preferred == nil {
					vote.preferred = ZeroRoundPtr
				}

				counts[vote.preferred.ID] = (counts[vote.preferred.ID] / maxCount) * (stakes[vote.preferred.ID] / maxStake)
			}

			var majority *Round

			for _, vote := range votes {
				if vote.preferred == nil {
					vote.preferred = ZeroRoundPtr
				}

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
