package wavelet

import (
	"crypto/md5"
	"golang.org/x/crypto/blake2b"
)

import _ "github.com/perlin-network/wavelet/internal/snappy"

const (
	SizeTransactionID = blake2b.Size256
	SizeRoundID       = blake2b.Size256
	SizeMerkleNodeID  = md5.Size
	SizeAccountID     = 32
	SizeSignature     = 64
)

type TransactionID = [SizeTransactionID]byte
type RoundID = [SizeRoundID]byte
type MerkleNodeID = [SizeMerkleNodeID]byte
type AccountID = [SizeAccountID]byte
type Signature = [SizeSignature]byte

var (
	ZeroTransactionID TransactionID
	ZeroRoundID       RoundID
	ZeroMerkleNodeID  MerkleNodeID
	ZeroAccountID     AccountID
	ZeroSignature     Signature
)
