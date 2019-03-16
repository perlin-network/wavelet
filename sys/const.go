package sys

import (
	"time"
)

// Transaction tags.
const (
	TagNop byte = iota
	TagTransfer
	TagContract
	TagStake
)

var (
	// Snowball consensus protocol parameters.
	SnowballQueryK     = 1
	SnowballQueryAlpha = 0.8
	SnowballQueryBeta  = 150

	SnowballSyncK     = 1
	SnowballSyncAlpha = 0.8
	SnowballSyncBeta  = 150

	// Timeout for querying a transaction to K peers.
	QueryTimeout = 10 * time.Second

	// Max graph depth difference to search for eligible transaction
	// parents from for our node.
	MaxEligibleParentsDepthDiff uint64 = 5

	// Minimum difficulty to define a critical transaction.
	MinDifficulty = 8

	// Maximum difficulty to define a critical transaction.
	MaxDifficulty = 12

	// Number of ancestors to derive a median timestamp from.
	MedianTimestampNumAncestors = 5

	TransactionFeeAmount uint64 = 2

	ExpectedConsensusTimeMilliseconds  uint64 = 3000
	CriticalTimestampAverageWindowSize        = 3

	MinimumStake uint64 = 100
)
