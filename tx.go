package wavelet

import (
	"bytes"
	"encoding/binary"
	"github.com/perlin-network/wavelet/common"
	"golang.org/x/crypto/blake2b"
	"math/bits"
)

type Transaction struct {
	ID       common.TransactionID // blake2b(*)
	Checksum uint64               // xxh3(ID)

	Sender    common.AccountID       // Transaction sender.
	ParentIDs []common.TransactionID // Transactions parents.

	Depth      uint64 // Graph depth.
	Confidence uint64 // Number of ancestors.
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

func (t Transaction) ExpectedDifficulty(min uint64) uint64 {
	log2 := func(x uint64) uint64 {
		return uint64(64 - bits.LeadingZeros64(x))
	}

	return min * log2(t.Confidence) / log2(t.Depth)
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
	var buf bytes.Buffer
	_, _ = buf.Write(tx.Sender[:])

	for _, parentID := range tx.ParentIDs {
		_, _ = buf.Write(parentID[:])
	}

	seed := blake2b.Sum256(buf.Bytes())

	return uint64(prefixLen(seed[:])) >= difficulty
}
