package wavelet

import (
	"bytes"
	"encoding/binary"
	"github.com/perlin-network/wavelet/common"
	"math"
	"math/bits"
)

type Transaction struct {
	Sender    common.AccountID       // Transaction sender.
	ParentIDs []common.TransactionID // Transactions parents.

	Depth      uint64 // Graph depth.
	Confidence uint64 // Number of ancestors.

	Tag     byte
	Payload []byte

	Signature common.Signature

	id       common.TransactionID // BLAKE2b(*).
	checksum uint64               // XXH3(ID).
	seed     byte                 // Number of prefixed zeroes of BLAKE2b(Sender || ParentIDs).
}

func (t Transaction) Marshal() []byte {
	w := bytes.NewBuffer(nil)

	w.Write(t.Sender[:])

	w.WriteByte(byte(len(t.ParentIDs)))
	for _, parentID := range t.ParentIDs {
		w.Write(parentID[:])
	}

	var buf [8]byte

	binary.BigEndian.PutUint64(buf[:8], t.Depth)
	w.Write(buf[:])

	binary.BigEndian.PutUint64(buf[:8], t.Confidence)
	w.Write(buf[:])

	return w.Bytes()
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

func (tx Transaction) IsCritical(difficulty uint64) bool {
	return uint64(tx.seed) >= difficulty
}
