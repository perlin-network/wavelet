package wavelet

import (
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
)

func computeStakeDistribution(tree *avl.Tree, accountIDs []common.AccountID, numAccounts int) map[common.AccountID]float64 {
	stakes := make(map[common.AccountID]uint64, len(accountIDs))
	var maxStake uint64

	for _, accountID := range accountIDs {
		stake, _ := ReadAccountStake(tree, accountID)

		if stake < sys.MinimumStake {
			stake = sys.MinimumStake
		}

		if maxStake < stake {
			maxStake = stake
		}

		stakes[accountID] = stake
	}

	weights := make(map[common.AccountID]float64, len(stakes))

	for account, stake := range stakes {
		weights[account] = float64(stake) / float64(maxStake) / float64(numAccounts)
	}

	return weights
}
