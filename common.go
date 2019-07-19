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

package wavelet

import (
	"crypto/md5"
	"golang.org/x/crypto/blake2b"
)

import _ "github.com/perlin-network/wavelet/internal/snappy"

const (
	SizeTransactionID   = blake2b.Size256
	SizeTransactionSeed = blake2b.Size256
	SizeRoundID         = blake2b.Size256
	SizeMerkleNodeID    = md5.Size
	SizeAccountID       = 32
	SizeSignature       = 64
)

type TransactionID = [SizeTransactionID]byte
type TransactionSeed = [SizeTransactionSeed]byte
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

	ZeroRoundPtr = &Round{}
)
