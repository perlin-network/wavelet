package params

import "time"

var (
	// Transaction IBLT parameters.
	TxK = 6
	TxL = 4096

	// Consensus protocol parameters.
	ConsensusK     = 1
	ConsensusAlpha = float32(0.8)

	// Ledger parameters.
	GraphUpdatePeriod = 100 * time.Millisecond

	// Sync parameters.
	SyncPeriod   = 1000 * time.Millisecond
	SyncNumPeers = 1
)
