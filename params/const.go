package params

import (
	"fmt"
)

const (
	// Transaction tags
	TagNop            = "nop"
	TagTransfer       = "transfer"
	TagPlaceStake     = "place_stake"
	TagWithdrawStake  = "withdraw_stake"
	TagCreateContract = "create_contract"

	// Account keys
	KeyContractCode = "contract_code"
)

var (
	// Consensus protocol parameters.
	ConsensusK     = 1
	ConsensusAlpha = float32(0.8)

	// Ledger parameters.
	GraphUpdatePeriodMs = 50

	// Sync parameters.

	// SyncHintPeriodMs is how often should we hint transactions we personally have to other nodes that might not have some of our transactions?
	SyncHintPeriodMs = 2000

	// SyncHintNumPeers is how many peers will we hint periodically about transactions they may potentially not hajve?
	SyncHintNumPeers = 3

	// SyncNeighborsLikelihood is if we receive a transaction we already have, give a 1/(this) probability we will ask nodes to see if we're missing any of its parents/children, and if so query them for it.
	SyncNeighborsLikelihood = 3

	// SyncNumPeers is how many peers will we query for missing transactions from?
	SyncNumPeers = 5

	// Reward parameters
	ValidatorRewardDepth     = 5
	ValidatorRewardAmount    = uint64(10)
	TransactionFeePercentage = 5
)

// DumpParams prints out all the params
func DumpParams() string {
	return fmt.Sprintf(`
  ConsensusK:               %v
  ConsensusAlpha:           %v
  GraphUpdatePeriodMs:      %v
  SyncHintPeriodMs:         %v
  SyncHintNumPeers:         %v
  SyncNeighborsLikelihood:  %v
  SyncNumPeers:             %v
  ValidatorRewardDepth:     %v
  ValidatorRewardAmount:    %v
  TransactionFeePercentage: %v
	`,
		ConsensusK,
		ConsensusAlpha,
		GraphUpdatePeriodMs,
		SyncHintPeriodMs,
		SyncHintNumPeers,
		SyncNeighborsLikelihood,
		SyncNumPeers,
		ValidatorRewardDepth,
		ValidatorRewardAmount,
		TransactionFeePercentage)
}
