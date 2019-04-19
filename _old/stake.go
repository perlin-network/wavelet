package _old

import (
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
)

func computeStakeDistribution(tree *avl.Tree, accounts map[common.AccountID]struct{}) map[common.AccountID]float64 {
	stakes := make(map[common.AccountID]uint64, len(accounts))
	var totalStake uint64

	for accountID := range accounts {
		stake, _ := ReadAccountStake(tree, accountID)

		if stake < sys.MinimumStake {
			stake = sys.MinimumStake
		}

		stakes[accountID] = stake
		totalStake += stake
	}

	weights := make(map[common.AccountID]float64, len(stakes))

	if totalStake == 0 {
		return weights
	}

	for account, stake := range stakes {
		weights[account] = float64(stake) / float64(totalStake)
	}

	return weights
}
