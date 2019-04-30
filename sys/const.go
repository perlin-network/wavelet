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
	// S/Kademlia overlay network parameters.
	SKademliaC1 = 16
	SKademliaC2 = 16

	// Snowball consensus protocol parameters.
	SnowballK     = 2
	SnowballAlpha = 0.8
	SnowballBeta  = 150

	// Timeout for querying a transaction to K peers.
	QueryTimeout = 1 * time.Second

	// Max graph depth difference to search for eligible transaction
	// parents from for our node.
	MaxDepthDiff uint64 = 5

	// Minimum difficulty to define a critical transaction.
	MinDifficulty = 10

	// Fee amount paid by a node per transaction.
	TransactionFeeAmount uint64 = 2

	// Minimum amount of stake to start being able to reap validator rewards.
	MinimumStake uint64 = 100

	GasTable = map[string]uint64{
		"nop":                 1,
		"unreachable":         1,
		"select":              120,
		"i32.const":           1,
		"i64.const":           1,
		"f32.const":           1,
		"f64.const":           1,
		"i32.add":             45,
		"i32.sub":             45,
		"i32.mul":             45,
		"i32.div_s":           45,
		"i32.div_u":           45,
		"i32.rem_s":           45,
		"i32.rem_u":           45,
		"i32.and":             45,
		"i32.or":              45,
		"i32.xor":             45,
		"i32.shl":             67,
		"i32.shr_s":           67,
		"i32.shr_u":           67,
		"i32.rotl":            90,
		"i32.rotr":            90,
		"i32.eq":              45,
		"i32.ne":              45,
		"i32.lt_s":            45,
		"i32.lt_u":            45,
		"i32.le_s":            45,
		"i32.le_u":            45,
		"i32.gt_s":            45,
		"i32.gt_u":            45,
		"i32.ge_u":            45,
		"i64.add":             45,
		"i64.sub":             45,
		"i64.mul":             45,
		"i64.div_s":           45,
		"i64.div_u":           45,
		"i64.rem_s":           45,
		"i64.rem_u":           45,
		"i64.and":             45,
		"i64.or":              45,
		"i64.xor":             45,
		"i64.shl":             67,
		"i64.shr_s":           67,
		"i64.shr_u":           67,
		"i64.rotl":            90,
		"i64.rotr":            90,
		"i64.eq":              45,
		"i64.ne":              45,
		"i64.lt_s":            45,
		"i64.lt_u":            45,
		"i64.le_s":            45,
		"i64.le_u":            45,
		"i64.gt_s":            45,
		"i64.gt_u":            45,
		"i64.ge_s":            45,
		"i64.ge_u":            45,
		"f32.add":             45,
		"f32.sub":             45,
		"f32.mul":             45,
		"f32.div":             45,
		"f32.min":             1,
		"f32.max":             1,
		"f32.copysign":        1,
		"f32.eq":              45,
		"f32.ne":              45,
		"f32.lt":              1,
		"f32.le":              1,
		"f32.gt":              1,
		"f32.ge":              1,
		"f64.add":             45,
		"f64.sub":             45,
		"f64.mul":             45,
		"f64.div":             45,
		"f64.min":             1,
		"f64.max":             1,
		"f64.copysign":        1,
		"f64.eq":              45,
		"f64.ne":              45,
		"f64.lt":              1,
		"f64.le":              1,
		"f64.gt":              1,
		"f64.ge":              1,
		"i32.ge_s":            45,
		"i32.clz":             45,
		"i32.ctz":             45,
		"i32.popcnt":          45,
		"i32.eqz":             45,
		"i64.clz":             45,
		"i64.ctz":             45,
		"i64.popcnt":          45,
		"i64.eqz":             45,
		"f32.sqrt":            1,
		"f32.ceil":            1,
		"f32.floor":           1,
		"f32.trunc":           1,
		"f32.nearest":         1,
		"f32.abs":             1,
		"f32.neg":             1,
		"f64.sqrt":            1,
		"f64.ceil":            1,
		"f64.floor":           1,
		"f64.trunc":           1,
		"f64.nearest":         1,
		"f64.abs":             1,
		"f64.neg":             1,
		"i32.wrap/i64":        1,
		"i64.extend_u/i32":    1,
		"i64.extend_s/i32":    1,
		"i32.trunc_u/f32":     1,
		"i32.trunc_u/f64":     1,
		"i64.trunc_u/f32":     1,
		"i64.trunc_u/f64":     1,
		"i32.trunc_s/f32":     1,
		"i32.trunc_s/f64":     1,
		"i64.trunc_s/f32":     1,
		"i64.trunc_s/f64":     1,
		"f32.demote/f64":      1,
		"f64.promote/f32":     1,
		"f32.convert_u/i32":   1,
		"f32.convert_u/i64":   1,
		"f64.convert_u/i32":   1,
		"f64.convert_u/i64":   1,
		"f32.convert_s/i32":   1,
		"f32.convert_s/i64":   1,
		"f64.convert_s/i32":   1,
		"f64.convert_s/i64":   1,
		"i32.reinterpret/f32": 1,
		"i64.reinterpret/f64": 1,
		"f32.reinterpret/i32": 1,
		"f64.reinterpret/i64": 1,
		"drop":                120,
		"i32.load":            120,
		"i64.load":            120,
		"i32.load8_s":         120,
		"i32.load16_s":        120,
		"i64.load8_s":         120,
		"i64.load16_s":        120,
		"i64.load32_s":        120,
		"i32.load8_u":         120,
		"i32.load16_u":        120,
		"i64.load8_u":         120,
		"i64.load16_u":        120,
		"i64.load32_u":        120,
		"f32.load":            120,
		"f64.load":            120,
		"i32.store":           120,
		"i32.store8":          120,
		"i32.store16":         120,
		"i64.store":           120,
		"i64.store8":          120,
		"i64.store16":         120,
		"i64.store32":         120,
		"f32.store":           120,
		"f64.store":           120,
		"get_local":           120,
		"get_global":          120,
		"set_local":           120,
		"set_global":          120,
		"tee_local":           120,
		"block":               1,
		"loop":                1,
		"if":                  1,
		"else":                1,
		"end":                 1,
		"br":                  1,
		"br_if":               1,
		"br_table":            1,
		"return":              1,
		"call":                90,
		"call_indirect":       10000,
		"current_memory":      100,
		"grow_memory":         10000,
	}
)
