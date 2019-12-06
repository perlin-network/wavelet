package wavelet

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

type Block struct {
	Index        uint64
	Merkle       MerkleNodeID
	Transactions []TransactionID

	ID BlockID
}

func NewBlock(index uint64, merkle MerkleNodeID, ids ...TransactionID) Block {
	b := Block{Index: index, Merkle: merkle, Transactions: ids}

	b.ID = blake2b.Sum256(b.Marshal())

	return b
}

func (b *Block) GetID() string {
	if b == nil || b.ID == ZeroBlockID {
		return ""
	}

	return fmt.Sprintf("%x", b.ID)
}

func (b Block) Marshal() []byte {
	buf := make([]byte, 8+SizeMerkleNodeID+4+len(b.Transactions)*SizeTransactionID)
	n := 0

	binary.BigEndian.PutUint64(buf[n:n+8], b.Index)

	n += 8

	copy(buf[n:n+SizeMerkleNodeID], b.Merkle[:])

	n += SizeMerkleNodeID

	binary.BigEndian.PutUint32(buf[n:n+4], uint32(len(b.Transactions)))

	n += 4

	for _, id := range b.Transactions {
		copy(buf[n:n+SizeTransactionID], id[:])

		n += SizeTransactionID
	}

	return buf
}

func (b Block) String() string {
	return hex.EncodeToString(b.ID[:])
}

func UnmarshalBlock(r io.Reader) (Block, error) {
	var (
		buf   [8]byte
		block Block
	)

	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return block, errors.Wrap(err, "failed to decode block index")
	}

	block.Index = binary.BigEndian.Uint64(buf[:8])

	if _, err := io.ReadFull(r, block.Merkle[:]); err != nil {
		return block, errors.Wrap(err, "failed to decode block's merkle root")
	}

	if _, err := io.ReadFull(r, buf[:4]); err != nil {
		return block, errors.Wrap(err, "failed to decode block's transactions length")
	}

	block.Transactions = make([]TransactionID, binary.BigEndian.Uint32(buf[:4]))

	for i := 0; i < len(block.Transactions); i++ {
		if _, err := io.ReadFull(r, block.Transactions[i][:]); err != nil {
			return block, errors.Wrap(err, "failed to decode one of the transactions")
		}
	}

	block.ID = blake2b.Sum256(block.Marshal())

	return block, nil
}
