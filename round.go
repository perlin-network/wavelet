package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"io"
)

// Nodes negotiate over which round to accept through Snowball. A round comprises of
// a Merkle root of the ledgers state proposed by the root transaction, alongside a
// single root transaction. Rounds are denoted by their ID, which is represented by
// BLAKE2b(merkle || root transactions content).
type Round struct {
	ID     common.RoundID
	Index  uint64
	Merkle common.MerkleNodeID
	Root   Transaction
}

func (r Round) Marshal() []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], r.Index)

	return append(buf[:], append(r.Merkle[:], r.Root.Marshal()...)...)
}

func UnmarshalRound(r io.Reader) (round Round, err error) {
	var buf [8]byte

	if _, err = io.ReadFull(r, buf[:]); err != nil {
		err = errors.Wrap(err, "failed to decode round index")
		return
	}

	round.Index = binary.BigEndian.Uint64(buf[:8])

	if _, err = io.ReadFull(r, round.Merkle[:]); err != nil {
		err = errors.Wrap(err, "failed to decode round merkle root")
		return
	}

	if round.Root, err = UnmarshalTransaction(r); err != nil {
		err = errors.Wrap(err, "failed to decode round root transaction")
		return
	}

	return
}

func NewRound(index uint64, merkle common.MerkleNodeID, root Transaction) Round {
	r := Round{
		Index:  index,
		Merkle: merkle,
		Root:   root,
	}

	r.ID = blake2b.Sum256(r.Marshal())

	return r
}
