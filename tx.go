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
	"io"
	"math/bits"
	"sort"

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

type Transaction struct {
	Sender AccountID // Transaction sender.

	Nonce uint64

	ParentIDs   []TransactionID // Transactions parents.
	ParentSeeds []TransactionSeed

	Depth uint64 // Graph depth.

	Tag     sys.Tag
	Payload []byte

	Signature Signature

	ID TransactionID // BLAKE2b(*).

	Seed    TransactionSeed // BLAKE2b(Sender || ParentIDs)
	SeedLen byte            // Number of prefixed zeroes of BLAKE2b(Sender || ParentIDs).
}

func NewTransaction(tag sys.Tag, payload []byte) Transaction {
	// var nonce [8]byte // TODO(kenta): nonce

	return Transaction{Tag: tag, Payload: payload}
}

// AttachSenderToTransaction immutably attaches sender to a transaction without modifying it in-place.
func AttachSenderToTransaction(sender *skademlia.Keypair, tx Transaction, parents ...*Transaction) Transaction {
	if len(parents) > 0 {
		tx.ParentIDs = make([]TransactionID, 0, len(parents))
		tx.ParentSeeds = make([]TransactionSeed, 0, len(parents))

		sort.Slice(parents, func(i, j int) bool {
			return bytes.Compare(parents[i].ID[:], parents[j].ID[:]) < 0
		})

		for _, parent := range parents {
			tx.ParentIDs = append(tx.ParentIDs, parent.ID)
			tx.ParentSeeds = append(tx.ParentSeeds, parent.Seed)

			if tx.Depth < parent.Depth {
				tx.Depth = parent.Depth
			}
		}

		tx.Depth++
	}

	tx.Sender = sender.PublicKey()
	tx.Signature = edwards25519.Sign(sender.PrivateKey(), tx.Marshal())

	tx.rehash()

	return tx
}

func (tx *Transaction) rehash() {
	logger := log.Node()

	tx.ID = blake2b.Sum256(tx.Marshal())

	// Calculate the new seed.

	hasher, err := blake2b.New(32, nil)
	if err != nil {
		logger.Fatal().Err(err).Msg("BUG: blake2b.New") // should never happen
	}

	_, err = hasher.Write(tx.Sender[:])
	if err != nil {
		logger.Fatal().Err(err).Msg("BUG: hasher.Write (1)") // should never happen
	}

	for _, parentSeed := range tx.ParentSeeds {
		_, err = hasher.Write(parentSeed[:])
		if err != nil {
			logger.Fatal().Err(err).Msg("BUG: hasher.Write (2)") // should never happen
		}
	}

	// Write 8-bit hash of transaction content to reduce conflicts.
	_, err = hasher.Write(tx.ID[:1])
	if err != nil {
		logger.Fatal().Err(err).Msg("BUG: hasher.Write (3)") // should never happen
	}

	seed := hasher.Sum(nil)
	copy(tx.Seed[:], seed)

	tx.SeedLen = byte(prefixLen(seed))
}

func (tx Transaction) ComputeSize() int {
	// TODO: optimize this
	return len(tx.Marshal())
}

func (tx Transaction) Marshal() []byte {
	w := bytes.NewBuffer(make([]byte, 0, 126+(SizeTransactionID*len(tx.ParentIDs))+SizeTransactionSeed*len(tx.ParentSeeds)+len(tx.Payload)))

	w.Write(tx.Sender[:])

	var buf [8]byte

	binary.BigEndian.PutUint64(buf[:8], tx.Nonce)
	w.Write(buf[:8])

	w.WriteByte(byte(len(tx.ParentIDs)))
	for _, parentID := range tx.ParentIDs {
		w.Write(parentID[:])
	}
	for _, parentSeed := range tx.ParentSeeds {
		w.Write(parentSeed[:])
	}

	binary.BigEndian.PutUint64(buf[:8], tx.Depth)
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

	if _, err = io.ReadFull(r, buf[:1]); err != nil {
		err = errors.Wrap(err, "failed to read num parents")
		return
	}

	if int(buf[0]) > sys.MaxParentsPerTransaction {
		err = errors.Errorf("tx while decoding has %d parents, but may only have at most %d parents", buf[0], sys.MaxParentsPerTransaction)
		return
	}

	t.ParentIDs = make([]TransactionID, buf[0])

	for i := range t.ParentIDs {
		if _, err = io.ReadFull(r, t.ParentIDs[i][:]); err != nil {
			err = errors.Wrapf(err, "failed to decode parent %d", i)
			return
		}
	}

	t.ParentSeeds = make([]TransactionSeed, len(t.ParentIDs))

	for i := range t.ParentSeeds {
		if _, err = io.ReadFull(r, t.ParentSeeds[i][:]); err != nil {
			err = errors.Wrapf(err, "failed to decode parent seed %d", i)
			return
		}
	}

	if _, err = io.ReadFull(r, buf[:8]); err != nil {
		err = errors.Wrap(err, "could not read transaction depth")
		return
	}

	t.Depth = binary.BigEndian.Uint64(buf[:8])

	if _, err = io.ReadFull(r, buf[:1]); err != nil {
		err = errors.Wrap(err, "could not read transaction tag")
		return
	}

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

	t.rehash()

	return t, nil
}

func (tx Transaction) IsCritical(difficulty byte) bool {
	return tx.SeedLen >= difficulty
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

func (tx Transaction) Fee() uint64 {
	fee := uint64(sys.TransactionFeeMultiplier * float64(tx.ComputeSize()))
	if fee < sys.DefaultTransactionFee {
		return sys.DefaultTransactionFee
	}

	return fee
}

func prefixLen(buf []byte) int {
	for i, b := range buf {
		if b != 0 {
			return i*8 + bits.LeadingZeros8(uint8(b))
		}
	}

	return len(buf)*8 - 1
}
