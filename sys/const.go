package sys

// Transaction tags.
const (
	TagNop byte = iota
	TagGeneric
	TagCreateContract
	TagStake
)

var (
	// Snowball consensus protocol parameters.
	SnowballK     = 1
	SnowballAlpha = float32(0.8)

	// Timeout for querying a transaction to K peers.
	QueryTimeout = 10000

	// Max graph depth difference to search for eligible transaction
	// parents from for our node.
	MaxEligibleParentsDepthDiff uint64 = 5

	// The max length of the entire entire history spanning
	// a specified amount of critical transactions.
	MaxHistoryWindowSize = 10
)
