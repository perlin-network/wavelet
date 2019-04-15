package common

import (
	"crypto/md5"
	"golang.org/x/crypto/blake2b"
)

const (
	SizeTransactionID = blake2b.Size256
	SizeMerkleNodeID  = md5.Size
	SizeAccountID     = 32
	SizeSignature     = 64
)

type TransactionID = [SizeTransactionID]byte
type MerkleNodeID = [SizeMerkleNodeID]byte
type AccountID = [SizeAccountID]byte
type Signature = [SizeSignature]byte

var (
	ZeroTransactionID TransactionID
	ZeroMerkleNodeID  MerkleNodeID
	ZeroAccountID     AccountID
	ZeroSignature     Signature
)
