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
	"math"
	"math/bits"
	"sort"
)

type Transaction struct {
	Sender  AccountID // Transaction sender.
	Creator AccountID // Transaction creator.

	Nonce uint64

	ParentIDs   []TransactionID // Transactions parents.
	ParentSeeds []TransactionSeed

	Depth uint64 // Graph depth.

	Tag     sys.Tag
	Payload []byte

	SenderSignature  Signature
	CreatorSignature Signature

	ID TransactionID // BLAKE2b(*).

	Seed    TransactionSeed // BLAKE2b(Sender || ParentIDs)
	SeedLen byte            // Number of prefixed zeroes of BLAKE2b(Sender || ParentIDs).
}

func NewTransaction(creator *skademlia.Keypair, tag sys.Tag, payload []byte) *Transaction {
	tx := &Transaction{Tag: tag, Payload: payload}

	var nonce [8]byte // TODO(kenta): nonce

	tx.Creator = creator.PublicKey()
	tx.CreatorSignature = edwards25519.Sign(creator.PrivateKey(), append(nonce[:], append([]byte{byte(tx.Tag)}, tx.Payload...)...))

	return tx
}

func NewBatchTransaction(creator *skademlia.Keypair, tags []byte, payloads [][]byte) *Transaction {
	if len(tags) != len(payloads) {
		panic("UNEXPECTED: Number of tags must be equivalent to number of payloads.")
	}

	if len(tags) == math.MaxUint8 {
		panic("UNEXPECTED: Total number of tags/payloads in a single batch transaction is 255.")
	}

	var size [4]byte
	var buf []byte

	for i := range tags {
		buf = append(buf, tags[i])

		binary.BigEndian.PutUint32(size[:4], uint32(len(payloads[i])))
		buf = append(buf, size[:4]...)
		buf = append(buf, payloads[i]...)
	}

	return NewTransaction(creator, sys.TagBatch, append([]byte{byte(len(tags))}, buf...))
}

func AttachSenderToTransaction(sender *skademlia.Keypair, _tx *Transaction, parents ...*Transaction) *Transaction {
	tx := *_tx
	AttachSenderToTransactionInPlace(sender, &tx, parents...)
	return &tx
}

func AttachSenderToTransactionInPlace(sender *skademlia.Keypair, tx *Transaction, parents ...*Transaction) {
	if len(parents) > 0 {
		tx.ParentIDs = make([]TransactionID, 0, len(parents))
		tx.ParentSeeds = make([]TransactionSeed, 0, len(parents))

		for _, parent := range parents {
			tx.ParentIDs = append(tx.ParentIDs, parent.ID)
			tx.ParentSeeds = append(tx.ParentSeeds, parent.Seed)

			if tx.Depth < parent.Depth {
				tx.Depth = parent.Depth
			}
		}

		tx.Depth++

		sort.Slice(tx.ParentIDs, func(i, j int) bool {
			return bytes.Compare(tx.ParentIDs[i][:], tx.ParentIDs[j][:]) < 0
		})
	}

	tx.Sender = sender.PublicKey()
	tx.SenderSignature = edwards25519.Sign(sender.PrivateKey(), tx.Marshal())

	tx.rehash()
}

func (t *Transaction) rehash() {
	t.ID = blake2b.Sum256(t.Marshal())

	// Calculate the new seed.
	{
		hasher, err := blake2b.New(32, nil)
		if err != nil {
			panic(err) // should never happen
		}

		_, err = hasher.Write(t.Sender[:])
		if err != nil {
			panic(err) // should never happen
		}

		for _, parentSeed := range t.ParentSeeds {
			_, err = hasher.Write(parentSeed[:])
			if err != nil {
				panic(err) // should never happen
			}
		}

		// Write 8-bit hash of transaction content to reduce conflicts.
		_, err = hasher.Write(t.ID[:1])
		if err != nil {
			panic(err) // should never happen
		}

		seed := hasher.Sum(nil)
		copy(t.Seed[:], seed)

		t.SeedLen = byte(prefixLen(seed))
	}
}

func (t *Transaction) Marshal() []byte {
	w := bytes.NewBuffer(make([]byte, 0, 222+(32*len(t.ParentIDs))+len(t.Payload)))

	w.Write(t.Sender[:])

	if t.Creator != t.Sender {
		w.WriteByte(1)
		w.Write(t.Creator[:])
	} else {
		w.WriteByte(0)
	}

	var buf [8]byte

	binary.BigEndian.PutUint64(buf[:8], t.Nonce)
	w.Write(buf[:8])

	w.WriteByte(byte(len(t.ParentIDs)))
	for _, parentID := range t.ParentIDs {
		w.Write(parentID[:])
	}
	for _, parentSeed := range t.ParentSeeds {
		w.Write(parentSeed[:])
	}

	binary.BigEndian.PutUint64(buf[:8], t.Depth)
	w.Write(buf[:8])

	w.WriteByte(byte(t.Tag))

	binary.BigEndian.PutUint32(buf[:4], uint32(len(t.Payload)))
	w.Write(buf[:4])
	w.Write(t.Payload)

	w.Write(t.SenderSignature[:])

	if t.Creator != t.Sender {
		w.Write(t.CreatorSignature[:])
	}

	return w.Bytes()
}

func UnmarshalTransaction(r io.Reader) (t *Transaction, err error) {
	t = &Transaction{}

	if _, err = io.ReadFull(r, t.Sender[:]); err != nil {
		err = errors.Wrap(err, "failed to decode transaction sender")
		return
	}

	var buf [8]byte

	if _, err = io.ReadFull(r, buf[:1]); err != nil {
		err = errors.Wrap(err, "failed to decode flag to see if transaction creator is recorded")
		return
	}

	if buf[0] != 0 && buf[0] != 1 {
		err = errors.Errorf("flag must be zero or one, but is %d instead", buf[0])
		return
	}

	creatorRecorded := buf[0] == 1

	if !creatorRecorded {
		t.Creator = t.Sender
	} else {
		if _, err = io.ReadFull(r, t.Creator[:]); err != nil {
			err = errors.Wrap(err, "failed to decode transaction creator")
			return
		}
	}

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

	if _, err = io.ReadFull(r, t.SenderSignature[:]); err != nil {
		err = errors.Wrap(err, "failed to decode signature")
		return
	}

	if !creatorRecorded {
		t.CreatorSignature = t.SenderSignature
	} else {
		if _, err = io.ReadFull(r, t.CreatorSignature[:]); err != nil {
			err = errors.Wrap(err, "failed to decode creator signature")
			return
		}
	}

	t.rehash()

	return t, nil
}

func (tx *Transaction) IsCritical(difficulty byte) bool {
	return tx.SeedLen >= difficulty
}

// LogicalUnits counts the total number of atomic logical units of changes
// the specified tx comprises of.
func (tx *Transaction) LogicalUnits() int {
	if tx.Tag != sys.TagBatch {
		return 1
	}

	var buf [1]byte

	if _, err := io.ReadFull(bytes.NewReader(tx.Payload), buf[:1]); err != nil {
		return 1
	}

	return int(buf[0])
}

func (tx *Transaction) String() string {
	return fmt.Sprintf("Transaction{ID: %x}", tx.ID)
}

func prefixLen(buf []byte) int {
	for i, b := range buf {
		if b != 0 {
			return i*8 + bits.LeadingZeros8(uint8(b))
		}
	}

	return len(buf)*8 - 1
}
