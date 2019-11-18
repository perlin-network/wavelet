// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

package sys

// Tag is a wrapper for a transaction tag.
type Tag byte

// Transaction tags.
const (
	_ Tag = iota // The first value was used for Nop. To make it backward compatible, we skip 0 value.
	TagTransfer
	TagContract
	TagStake
	TagBatch
)

const (
	WithdrawStake byte = iota
	PlaceStake
	WithdrawReward
)

const (
	// Size of individual chunks sent for a syncing peer.
	SyncChunkSize = 16 * 1024 // 64KB

	// Size of file size used for streaming file to disk during syncing.
	SyncPooledFileSize = 100 * 1024 * 1024 // 100MB
)

var (
	// SKademliaC1 and SKademliaC2 - S/Kademlia overlay network parameters.
	SKademliaC1 = 1
	SKademliaC2 = 1

	// DefaultTransactionFee Default fee amount paid by a node per transaction.
	DefaultTransactionFee uint64 = 2

	// TransactionFeeMultiplier Multiplier for size of transaction payload to calculate it's fee
	TransactionFeeMultiplier = 0.05

	// MinimumStake Minimum amount of stake to start being able to reap validator rewards.
	MinimumStake uint64 = 100

	MinimumRewardWithdraw = MinimumStake

	RewardWithdrawalsBlockLimit = 50

	FaucetAddress = "0f569c84d434fb0ca682c733176f7c0c2d853fce04d95ae131d2f9b4124d93d8"

	TagLabels = map[string]Tag{
		`transfer`: TagTransfer,
		`contract`: TagContract,
		`batch`:    TagBatch,
		`stake`:    TagStake,
	}

	ContractDefaultMemoryPages = 4
	ContractMaxMemoryPages     = 4096
	ContractTableSize          = 4096
	ContractMaxValueSlots      = 8192
	ContractMaxCallStackDepth  = 256
	ContractMaxGlobals         = 64
)

func init() { // nolint:gochecknoinits
	if VersionMeta == "testnet" {
		MinimumStake = 10000
	}
}

// String converts a given tag to a string.
func (tag Tag) String() string {
	if tag < 0 || tag > 3 { // nolint:staticcheck
		return "" // Return invalid tag
	}

	return []string{"transfer", "contract", "stake", "batch"}[tag] // Return tag
}
