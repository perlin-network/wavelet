package wavelet

import (
	"bytes"
	"encoding/binary"
	"github.com/dgryski/go-xxh3"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"io"
	"math"
	"math/bits"
)

type Transaction struct {
	Sender  common.AccountID // Transaction sender.
	Creator common.AccountID // Transaction creator.

	Nonce uint64

	ParentIDs []common.TransactionID // Transactions parents.

	Depth      uint64 // Graph depth.
	Confidence uint64 // Number of ancestors.

	Tag     byte
	Payload []byte

	SenderSignature  common.Signature
	CreatorSignature common.Signature

	ID       common.TransactionID // BLAKE2b(*).
	Checksum uint64               // XXH3(ID).
	Seed     byte                 // Number of prefixed zeroes of BLAKE2b(Sender || ParentIDs).
}

func (t *Transaction) rehash() *Transaction {
	t.ID = blake2b.Sum256(t.Marshal())
	t.Checksum = xxh3.XXH3_64bits(t.ID[:])

	buf := make([]byte, 0, common.SizeAccountID+len(t.ParentIDs)*common.SizeTransactionID)
	buf = append(buf, t.Sender[:]...)
	for _, parentID := range t.ParentIDs {
		buf = append(buf, parentID[:]...)
	}

	seed := blake2b.Sum256(buf)
	t.Seed = byte(prefixLen(seed[:]))

	return t
}

func (t Transaction) Marshal() []byte {
	w := bytes.NewBuffer(nil)

	w.Write(t.Sender[:])
	w.Write(t.Creator[:])

	var buf [8]byte

	binary.BigEndian.PutUint64(buf[:8], t.Nonce)
	w.Write(buf[:8])

	w.WriteByte(byte(len(t.ParentIDs)))
	for _, parentID := range t.ParentIDs {
		w.Write(parentID[:])
	}

	binary.BigEndian.PutUint64(buf[:8], t.Depth)
	w.Write(buf[:8])

	binary.BigEndian.PutUint64(buf[:8], t.Confidence)
	w.Write(buf[:8])

	w.WriteByte(t.Tag)

	binary.BigEndian.PutUint32(buf[:4], uint32(len(t.Payload)))
	w.Write(buf[:4])
	w.Write(t.Payload)

	w.Write(t.SenderSignature[:])
	w.Write(t.CreatorSignature[:])

	return w.Bytes()
}

func UnmarshalTransaction(r io.Reader) (t Transaction, err error) {
	if _, err = io.ReadFull(r, t.Sender[:]); err != nil {
		err = errors.Wrap(err, "failed to decode transaction sender")
		return
	}

	if _, err = io.ReadFull(r, t.Creator[:]); err != nil {
		err = errors.Wrap(err, "failed to decode transaction creator")
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

	t.ParentIDs = make([]common.TransactionID, buf[0])

	for i := range t.ParentIDs {
		if _, err = io.ReadFull(r, t.ParentIDs[i][:]); err != nil {
			err = errors.Wrapf(err, "failed to decode parent %d", i)
			return
		}
	}

	if _, err = io.ReadFull(r, buf[:8]); err != nil {
		err = errors.Wrap(err, "could not read transaction depth")
		return
	}

	t.Depth = binary.BigEndian.Uint64(buf[:8])

	if _, err = io.ReadFull(r, buf[:8]); err != nil {
		err = errors.Wrap(err, "could not read transaction confidence")
		return
	}

	t.Confidence = binary.BigEndian.Uint64(buf[:8])

	if _, err = io.ReadFull(r, buf[:1]); err != nil {
		err = errors.Wrap(err, "could not read transaction tag")
		return
	}

	t.Tag = buf[0]

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

	if _, err = io.ReadFull(r, t.CreatorSignature[:]); err != nil {
		err = errors.Wrap(err, "failed to decode creator signature")
		return
	}

	t.rehash()

	return t, nil
}

func (t Transaction) ExpectedDifficulty(min byte) byte {
	if t.Depth == 0 && t.Confidence == 0 {
		return min
	}

	log2 := func(x uint64) uint64 {
		return uint64(64 - bits.LeadingZeros64(x))
	}

	mul := func(a, b uint64) uint64 {
		c := a * b

		if c/b != a {
			c = math.MaxUint64
		}

		return c
	}

	difficulty := byte(mul(uint64(min), log2(t.Confidence)) / log2(t.Depth))

	if difficulty < min {
		difficulty = min
	}

	return difficulty
}

func prefixLen(buf []byte) int {
	for i, b := range buf {
		if b != 0 {
			return i*8 + bits.LeadingZeros8(uint8(b))
		}
	}

	return len(buf)*8 - 1
}

func (tx Transaction) IsCritical(difficulty byte) bool {
	return tx.Seed >= difficulty
}
