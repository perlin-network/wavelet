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

	"github.com/perlin-network/noise/edwards25519"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"github.com/valyala/fastjson"
	"golang.org/x/crypto/blake2b"
)

type Transaction struct {
	ID     TransactionID `json:"id"`     // BLAKE2b(*).
	Sender AccountID     `json:"sender"` // Transaction sender.

	Nonce uint64 `json:"nonce"`
	Block uint64 `json:"block"`

	Tag     sys.Tag `json:"tag"`
	Payload []byte  `json:"-"`

	Signature Signature `json:"-"`

	Error error `json:"error,omitempty"`
}

var _ log.Loggable = (*Transaction)(nil)

func NewTransaction(sender *skademlia.Keypair, nonce, block uint64,
	tag sys.Tag, payload []byte) Transaction {

	var nonceBuf [8]byte
	binary.BigEndian.PutUint64(nonceBuf[:], nonce)

	var blockBuf [8]byte
	binary.BigEndian.PutUint64(blockBuf[:], block)

	signature := edwards25519.Sign(
		sender.PrivateKey(),
		append(nonceBuf[:], append(blockBuf[:], append(
			[]byte{byte(tag)}, payload...,
		)...)...),
	)

	return NewSignedTransaction(sender.PublicKey(), nonce, block, tag, payload, signature)
}

func NewSignedTransaction(sender edwards25519.PublicKey, nonce, block uint64,
	tag sys.Tag, payload []byte, signature edwards25519.Signature) Transaction {

	tx := Transaction{
		Sender:    sender,
		Nonce:     nonce,
		Block:     block,
		Tag:       tag,
		Payload:   payload,
		Signature: signature,
	}

	tx.ID = blake2b.Sum256(tx.Marshal())

	return tx
}

func (tx *Transaction) MarshalEvent(ev *zerolog.Event) {
	if tx.Error != nil {
		ev.Err(tx.Error)
	}

	ev.Hex("id", tx.ID[:])
	ev.Hex("sender", tx.Sender[:])
	ev.Uint64("nonce", tx.Nonce)
	ev.Uint64("block", tx.Block)
	ev.Uint8("tag", uint8(tx.Tag))
	ev.Msg("Transaction")
}

func (tx *Transaction) UnmarshalValue(v *fastjson.Value) error {
	log.ValueHex(v, tx.ID, "id")
	log.ValueHex(v, tx.Sender, "sender")
	tx.Nonce = v.GetUint64("nonce")
	tx.Block = v.GetUint64("block")
	tx.Tag = sys.Tag(v.GetUint("tag"))

	if err := v.GetStringBytes("error"); len(err) > 0 {
		tx.Error = errors.New(string(err))
	}

	return nil
}

func (tx *Transaction) Marshal() []byte {
	w := bytes.NewBuffer(make([]byte, 0, 32+8+8+1+4+len(tx.Payload)+64))

	w.Write(tx.Sender[:])

	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:8], tx.Nonce)
	w.Write(buf[:8])

	binary.BigEndian.PutUint64(buf[:8], tx.Block)
	w.Write(buf[:8])

	w.WriteByte(byte(tx.Tag))

	binary.BigEndian.PutUint32(buf[:4], uint32(len(tx.Payload)))
	w.Write(buf[:4])

	w.Write(tx.Payload)

	w.Write(tx.Signature[:])

	return w.Bytes()
}

func (t *Transaction) Unmarshal(r io.Reader) (err error) {
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

	if _, err = io.ReadFull(r, buf[:8]); err != nil {
		err = errors.Wrap(err, "failed to read block index")
		return
	}

	t.Block = binary.BigEndian.Uint64(buf[:8])

	if _, err = io.ReadFull(r, buf[:1]); err != nil {
		err = errors.Wrap(err, "failed to read tag")
		return
	}

	t.Tag = sys.Tag(buf[0])

	if t.Tag < sys.TagTransfer || t.Tag > sys.TagBatch {
		err = errors.Wrapf(err, "got an unknown tag %d", t.Tag)
		return
	}

	if _, err = io.ReadFull(r, buf[:4]); err != nil {
		err = errors.Wrap(err, "could not read transaction payload length")
		return
	}

	t.Payload = make([]byte, binary.BigEndian.Uint32(buf[:4]))

	if _, err = io.ReadFull(r, t.Payload); err != nil {
		err = errors.Wrap(err, "could not read transaction payload")
		return
	}

	if _, err = io.ReadFull(r, t.Signature[:]); err != nil {
		err = errors.Wrap(err, "failed to decode signature")
		return
	}

	t.ID = blake2b.Sum256(t.Marshal())

	return nil
}

func UnmarshalTransaction(r io.Reader) (Transaction, error) {
	var t Transaction
	return t, t.Unmarshal(r)
}

func (tx Transaction) ComputeIndex(id BlockID) []byte {
	idx := blake2b.Sum256(append(tx.ID[:], id[:]...))
	return idx[:]
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

func (tx Transaction) VerifySignature() bool {
	var (
		nonceBuf [8]byte
		blockBuf [8]byte
	)

	binary.BigEndian.PutUint64(nonceBuf[:], tx.Nonce)
	binary.BigEndian.PutUint64(blockBuf[:], tx.Block)

	message := make([]byte, 0, 8+8+1+len(tx.Payload))
	message = append(message, nonceBuf[:]...)
	message = append(message, blockBuf[:]...)
	message = append(message, byte(tx.Tag))
	message = append(message, tx.Payload...)

	return edwards25519.Verify(tx.Sender, message, tx.Signature)
}
