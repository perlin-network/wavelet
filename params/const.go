package params

import "time"

var (
	// Transaction IBLT parameters.
	TxK = 6
	TxL = 4096*8

	// Consensus protocol parameters.
	ConsensusK     = 3
	ConsensusAlpha = float32(0.8)

	// Ledger parameters.
	GraphUpdatePeriod = 50 * time.Millisecond

	// Sync parameters.
	SyncPeriod   = 4000 * time.Millisecond
	SyncNumPeers = 1
)
