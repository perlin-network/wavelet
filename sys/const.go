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
	SnowballK     = 1
	SnowballAlpha = 0.8
	SnowballBeta  = 10

	// Timeout for querying a transaction to K peers.
	QueryTimeout = 10 * time.Second

	// Max graph depth difference to search for eligible transaction
	// parents from for our node.
	MaxEligibleParentsDepthDiff uint64 = 5

	// Minimum difficulty to define a critical transaction.
	MinDifficulty = 5

	// Maximum difficulty to define a critical transaction.
	MaxDifficulty = 16

	// Number of ancestors to derive a median timestamp from.
	MedianTimestampNumAncestors = 5

	ValidatorRewardAmount uint64 = 2

	ExpectedConsensusTimeMilliseconds  uint64 = 1000
	CriticalTimestampAverageWindowSize        = 3

	MinimumStake uint64 = 100
)
