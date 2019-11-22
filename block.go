package wavelet

import (
	"bytes"
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

func NewBlock(index uint64, merkle MerkleNodeID, ids ...TransactionID) (Block, error) {
	b := Block{Index: index, Merkle: merkle, Transactions: ids}

	marshaled, err := b.Marshal()
	if err != nil {
		return b, errors.Wrap(err, "failed to marshal block")
	}

	b.ID = blake2b.Sum256(marshaled)

	return b, nil
}

func (b *Block) GetID() string {
	if b == nil || b.ID == ZeroBlockID {
		return ""
	}

	return fmt.Sprintf("%x", b.ID)
}

func (b Block) Marshal() ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 8+SizeMerkleNodeID+4+4+len(b.Transactions)*SizeTransactionID))

	if err := binary.Write(buf, binary.BigEndian, b.Index); err != nil {
		return nil, errors.Wrap(err, "error marshaling index")
	}

	buf.Write(b.Merkle[:])

	if err := binary.Write(buf, binary.BigEndian, uint32(len(b.Transactions))); err != nil {
		return nil, errors.Wrap(err, "error marshaling transactions num")
	}

	for _, id := range b.Transactions {
		buf.Write(id[:])
	}

	return buf.Bytes(), nil
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

	marshaled, err := block.Marshal()
	if err != nil {
		return block, errors.Wrap(err, "failed to marshal block")
	}

	block.ID = blake2b.Sum256(marshaled)

	return block, nil
}
