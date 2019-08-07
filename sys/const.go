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

import "time"

// Tag is a wrapper for a transaction tag.
type Tag byte

// Transaction tags.
const (
	TagNop Tag = iota
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

var (
	// S/Kademlia overlay network parameters.
	SKademliaC1 = 1
	SKademliaC2 = 1

	// Snowball consensus protocol parameters.
	SnowballK     = 2
	SnowballAlpha = 0.8
	SnowballBeta  = 150

	// Timeout for querying a transaction to K peers.
	QueryTimeout = 1 * time.Second

	// Number of rounds we should be behind before we start syncing.
	SyncIfRoundsDifferBy uint64 = 2

	// Size of individual chunks sent for a syncing peer.
	SyncChunkSize = 16384

	// Max graph depth difference to search for eligible transaction
	// parents from for our node.
	MaxDepthDiff uint64 = 10

	// Max number of parents referencable by a transaction.
	MaxParentsPerTransaction = 32

	// Minimum difficulty to define a critical transaction.
	MinDifficulty byte = 8

	// Factor to scale a transactions confidence down by to compute the difficulty needed to define a critical transaction.
	DifficultyScaleFactor = 0.5

	// Fee amount paid by a node per transaction.
	TransactionFeeAmount uint64 = 2

	// Minimum amount of stake to start being able to reap validator rewards.
	MinimumStake uint64 = 100

	MinimumRewardWithdraw = MinimumStake

	RewardWithdrawalsRoundLimit = 50

	PruningLimit = uint8(30)

	FaucetAddress = "0f569c84d434fb0ca682c733176f7c0c2d853fce04d95ae131d2f9b4124d93d8"

	GasTable = map[string]uint64{
		"nop":                     1,
		"unreachable":             1,
		"select":                  12,
		"i32.const":               1,
		"i64.const":               1,
		"f32.const":               1,
		"f64.const":               1,
		"i32.add":                 5,
		"i32.sub":                 5,
		"i32.mul":                 5,
		"i32.div_s":               7,
		"i32.div_u":               7,
		"i32.rem_s":               7,
		"i32.rem_u":               7,
		"i32.and":                 5,
		"i32.or":                  5,
		"i32.xor":                 5,
		"i32.shl":                 7,
		"i32.shr_s":               7,
		"i32.shr_u":               7,
		"i32.rotl":                9,
		"i32.rotr":                9,
		"i32.eq":                  5,
		"i32.ne":                  5,
		"i32.lt_s":                5,
		"i32.lt_u":                5,
		"i32.le_s":                5,
		"i32.le_u":                5,
		"i32.gt_s":                5,
		"i32.gt_u":                5,
		"i32.ge_u":                5,
		"i64.add":                 5,
		"i64.sub":                 5,
		"i64.mul":                 5,
		"i64.div_s":               5,
		"i64.div_u":               5,
		"i64.rem_s":               5,
		"i64.rem_u":               5,
		"i64.and":                 5,
		"i64.or":                  5,
		"i64.xor":                 5,
		"i64.shl":                 7,
		"i64.shr_s":               7,
		"i64.shr_u":               7,
		"i64.rotl":                9,
		"i64.rotr":                9,
		"i64.eq":                  5,
		"i64.ne":                  5,
		"i64.lt_s":                5,
		"i64.lt_u":                5,
		"i64.le_s":                5,
		"i64.le_u":                5,
		"i64.gt_s":                5,
		"i64.gt_u":                5,
		"i64.ge_s":                5,
		"i64.ge_u":                5,
		"f32.add":                 5,
		"f32.sub":                 5,
		"f32.mul":                 5,
		"f32.div":                 5,
		"f32.min":                 5,
		"f32.max":                 5,
		"f32.copysign":            5,
		"f32.eq":                  5,
		"f32.ne":                  5,
		"f32.lt":                  5,
		"f32.le":                  5,
		"f32.gt":                  5,
		"f32.ge":                  5,
		"f64.add":                 5,
		"f64.sub":                 5,
		"f64.mul":                 5,
		"f64.div":                 5,
		"f64.min":                 5,
		"f64.max":                 5,
		"f64.copysign":            5,
		"f64.eq":                  5,
		"f64.ne":                  5,
		"f64.lt":                  5,
		"f64.le":                  5,
		"f64.gt":                  5,
		"f64.ge":                  5,
		"i32.ge_s":                5,
		"i32.clz":                 5,
		"i32.ctz":                 5,
		"i32.popcnt":              5,
		"i32.eqz":                 5,
		"i64.clz":                 5,
		"i64.ctz":                 5,
		"i64.popcnt":              5,
		"i64.eqz":                 5,
		"f32.sqrt":                9,
		"f32.ceil":                9,
		"f32.floor":               9,
		"f32.trunc":               9,
		"f32.nearest":             9,
		"f32.abs":                 9,
		"f32.neg":                 9,
		"f64.sqrt":                9,
		"f64.ceil":                9,
		"f64.floor":               9,
		"f64.trunc":               9,
		"f64.nearest":             9,
		"f64.abs":                 9,
		"f64.neg":                 9,
		"i32.wrap/i64":            5,
		"i64.extend_u/i32":        7,
		"i64.extend_s/i32":        7,
		"i32.trunc_u/f32":         7,
		"i32.trunc_u/f64":         7,
		"i64.trunc_u/f32":         7,
		"i64.trunc_u/f64":         7,
		"i32.trunc_s/f32":         7,
		"i32.trunc_s/f64":         7,
		"i64.trunc_s/f32":         7,
		"i64.trunc_s/f64":         7,
		"f32.demote/f64":          7,
		"f64.promote/f32":         7,
		"f32.convert_u/i32":       7,
		"f32.convert_u/i64":       7,
		"f64.convert_u/i32":       7,
		"f64.convert_u/i64":       7,
		"f32.convert_s/i32":       7,
		"f32.convert_s/i64":       7,
		"f64.convert_s/i32":       7,
		"f64.convert_s/i64":       7,
		"i32.reinterpret/f32":     5,
		"i64.reinterpret/f64":     5,
		"f32.reinterpret/i32":     5,
		"f64.reinterpret/i64":     5,
		"drop":                    12,
		"i32.load":                12,
		"i64.load":                12,
		"i32.load8_s":             12,
		"i32.load16_s":            12,
		"i64.load8_s":             12,
		"i64.load16_s":            12,
		"i64.load32_s":            12,
		"i32.load8_u":             12,
		"i32.load16_u":            12,
		"i64.load8_u":             12,
		"i64.load16_u":            12,
		"i64.load32_u":            12,
		"f32.load":                12,
		"f64.load":                12,
		"i32.store":               12,
		"i32.store8":              12,
		"i32.store16":             12,
		"i64.store":               12,
		"i64.store8":              12,
		"i64.store16":             12,
		"i64.store32":             12,
		"f32.store":               12,
		"f64.store":               12,
		"get_local":               12,
		"get_global":              12,
		"set_local":               12,
		"set_global":              12,
		"tee_local":               12,
		"block":                   1,
		"loop":                    1,
		"if":                      1,
		"else":                    1,
		"end":                     1,
		"br":                      1,
		"br_if":                   1,
		"br_table":                1,
		"return":                  1,
		"call":                    9,
		"call_indirect":           100,
		"current_memory":          10,
		"grow_memory":             1000,
		"wavelet.hash.blake2b256": 1500, // TODO: Review
		"wavelet.hash.blake2b512": 2000, // TODO: Review
		"wavelet.hash.sha256":     2500, // TODO: Review
		"wavelet.hash.sha512":     3000, // TODO: Review
		"wavelet.verify.ed25519":  5000, // TODO: Review
	}

	TagLabels = map[string]Tag{
		`nop`:      TagNop,
		`transfer`: TagTransfer,
		`contract`: TagContract,
		`batch`:    TagBatch,
		`stake`:    TagStake,
	}

	ContractDefaultMemoryPages = 4
	ContractMaxMemoryPages     = 32
	ContractTableSize          = 4096
	ContractMaxValueSlots      = 8192
	ContractMaxCallStackDepth  = 256
	ContractMaxGlobals         = 64
)

// String converts a given tag to a string.
func (tag Tag) String() string {
	if tag < 0 || tag > 4 { // Check out of bounds
		return "" // Return invalid tag
	}

	return []string{"nop", "transfer", "contract", "stake", "batch"}[tag] // Return tag
}
