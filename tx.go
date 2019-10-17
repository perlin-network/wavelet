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
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"io"
	"math/big"
)

type Transaction struct {
	Sender AccountID // Transaction sender.

	Nonce uint64

	Tag     sys.Tag
	Payload []byte

	Signature Signature

	ID TransactionID // BLAKE2b(*).
}

func NewTransaction(sender *skademlia.Keypair, tag sys.Tag, payload []byte) Transaction {
	// var nonce [8]byte // TODO(kenta): nonce

	tx := Transaction{Sender: sender.PublicKey(), Tag: tag, Payload: payload}
	tx.Signature = edwards25519.Sign(sender.PrivateKey(), tx.Marshal())

	tx.ID = blake2b.Sum256(tx.Marshal())

	return tx
}

func (tx Transaction) Marshal() []byte {
	var w bytes.Buffer

	w.Write(tx.Sender[:])

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:8], tx.Nonce)
	w.Write(buf[:8])

	w.WriteByte(byte(tx.Tag))

	binary.BigEndian.PutUint32(buf[:4], uint32(len(tx.Payload)))
	w.Write(buf[:4])

	w.Write(tx.Payload)

	w.Write(tx.Signature[:])

	return w.Bytes()
}

func UnmarshalTransaction(r io.Reader) (t Transaction, err error) {
	if _, err = io.ReadFull(r, t.Sender[:]); err != nil {
		err = errors.Wrap(err, "failed to decode transaction sender")
		return
	}

	var buf [8]byte

	if _, err = io.ReadFull(r, buf[:8]); err != nil {
		err = errors.Wrap(err, "failed to read nonce")
		return
	}

	t.Nonce = binary.BigEndian.Uint64(buf[:8])

	t.Tag = sys.Tag(buf[0])

	if _, err = io.ReadFull(r, buf[:4]); err != nil {
		err = errors.Wrap(err, "could not read transaction payload length")
		return
	}

	t.Payload = make([]byte, binary.BigEndian.Uint32(buf[:4]))

	if _, err = io.ReadFull(r, t.Payload[:]); err != nil {
		err = errors.Wrap(err, "could not read transaction payload")
		return
	}

	if _, err = io.ReadFull(r, t.Signature[:]); err != nil {
		err = errors.Wrap(err, "failed to decode signature")
		return
	}

	t.ID = blake2b.Sum256(t.Marshal())

	return t, nil
}

func (tx Transaction) ComputeIndex(id BlockID) *big.Int {
	buf := blake2b.Sum256(append(tx.ID[:], id[:]...))
	index := (&big.Int{}).SetBytes(buf[:])

	return index
}

func (tx Transaction) Fee() uint64 {
	fee := uint64(sys.TransactionFeeMultiplier * float64(len(tx.Payload)))
	if fee < sys.DefaultTransactionFee {
		return sys.DefaultTransactionFee
	}

	return fee
}

// LogicalUnits counts the total number of atomic logical units of changes
// the specified tx comprises of.
func (tx Transaction) LogicalUnits() int {
	if tx.Tag != sys.TagBatch {
		return 1
	}

	var buf [1]byte

	if _, err := io.ReadFull(bytes.NewReader(tx.Payload), buf[:1]); err != nil {
		return 1
	}

	return int(buf[0])
}

func (tx Transaction) String() string {
	return fmt.Sprintf("Transaction{ID: %x}", tx.ID)
}
