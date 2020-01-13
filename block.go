package wavelet

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"reflect"
	"unsafe"

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

func (b *Block) GetID() string {
	if b == nil || b.ID == ZeroBlockID {
		return ""
	}

	return fmt.Sprintf("%x", b.ID)
}

func (b Block) Marshal() []byte {
	buf, n := make([]byte, 8+SizeMerkleNodeID+4+len(b.Transactions)*SizeTransactionID), 0

	binary.BigEndian.PutUint64(buf[n:n+8], b.Index)

	n += 8

	copy(buf[n:n+SizeMerkleNodeID], b.Merkle[:])

	n += SizeMerkleNodeID

	binary.BigEndian.PutUint32(buf[n:n+4], uint32(len(b.Transactions)))

	n += 4

	// Directly convert the slice of transaction IDs into a byte slice through
	// pointer re-alignment. This is done by taking the internal pointer of
	// (*Block).Transactions, and re-assigning the pointer to an empty
	// byte slice.

	{
		var ids []byte

		sh := (*reflect.SliceHeader)(unsafe.Pointer(&ids))
		sh.Data = (*reflect.SliceHeader)(unsafe.Pointer(&b.Transactions)).Data
		sh.Len = SizeTransactionID * len(b.Transactions)
		sh.Cap = SizeTransactionID * len(b.Transactions)

		copy(buf[n:n+len(b.Transactions)*SizeTransactionID], ids)
	}

	return buf
}

func (b Block) String() string {
	return hex.EncodeToString(b.ID[:])
}
