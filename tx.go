package wavelet

import (
	"bytes"
	"github.com/perlin-network/wavelet/common"
	"golang.org/x/crypto/blake2b"
	"math/bits"
)

type Transaction struct {
	ID       common.TransactionID
	Checksum uint64 // xxh3(*)

	Sender    common.AccountID
	ParentIDs []common.TransactionID

	Round uint64
	Depth uint64
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
