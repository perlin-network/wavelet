package common

import (
	"golang.org/x/crypto/blake2b"
)

const (
	SizeTransactionID = blake2b.Size256
	SizeAccountID     = 32
	SizeSignature     = 64
)

type TransactionID = [SizeTransactionID]byte
type AccountID = [SizeAccountID]byte

type Signature = [SizeSignature]byte

var (
	ZeroTransactionID TransactionID
	ZeroAccountID     AccountID
	ZeroSignature     Signature
)
