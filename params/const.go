package params

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

	ValidatorRewardDepth  = 5
	ValidatorRewardAmount = uint64(10)
)
