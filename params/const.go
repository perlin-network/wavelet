package params

import "time"

const (
	// Transaction tags
	TagCreateContract = "create_contract"
	TagNop            = "nop"
	TagTransfer       = "transfer"

	// Account keys
	KeyContractCode = "contract_code"
)

var (
	// Transaction IBLT parameters.
	TxK = 6
	TxL = 4096

	// Consensus protocol parameters.
	ConsensusK     = 1
	ConsensusAlpha = float32(0.8)

	// Ledger parameters.
	GraphUpdatePeriod = 50 * time.Millisecond

	// Sync parameters.

	// How often should we hint transactions we personally have to other nodes that might not have some of our transactions?
	SyncHintPeriod = 2000 * time.Millisecond

	// How many peers will we hint periodically about transactions they may potentially not hajve?
	SyncHintNumPeers = 3

	// If we receive a transaction we already have, give a 1/(this) probability we will ask nodes to see if we're missing any of its parents/children, and if so query them for it.
	SyncNeighborsLikelihood = 3

	// How many peers will we query for missing transactions from?
	SyncNumPeers = 5
)
