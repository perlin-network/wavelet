package wavelet

import (
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
)

func ComputeStakeDistribution(a accounts, accountIDs []common.AccountID) map[common.AccountID]float64 {
	stakes := make(map[common.AccountID]uint64)
	var maxStake uint64

	for _, accountID := range accountIDs {
		stake, _ := a.ReadAccountStake(accountID)

		if stake < sys.MinimumStake {
			stake = sys.MinimumStake
		}

		if maxStake < stake {
			maxStake = stake
		}

		stakes[accountID] = stake
	}

	weights := make(map[common.AccountID]float64)

	for account, stake := range stakes {
		weights[account] = float64(stake) / float64(maxStake) / float64(sys.SnowballQueryK)
	}

	return weights
}
