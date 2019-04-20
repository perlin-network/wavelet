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
	id     common.RoundID
	idx    uint64
	merkle common.MerkleNodeID
	root   Transaction
}

func (r Round) Marshal() []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], r.idx)

	return append(buf[:], append(r.merkle[:], r.root.Marshal()...)...)
}

func UnmarshalRound(r io.Reader) (round Round, err error) {
	var buf [8]byte

	if _, err = io.ReadFull(r, buf[:]); err != nil {
		err = errors.Wrap(err, "failed to decode round index")
		return
	}

	round.idx = binary.BigEndian.Uint64(buf[:8])

	if _, err = io.ReadFull(r, round.merkle[:]); err != nil {
		err = errors.Wrap(err, "failed to decode round merkle root")
		return
	}

	if round.root, err = UnmarshalTransaction(r); err != nil {
		err = errors.Wrap(err, "failed to decode round root transaction")
		return
	}

	return
}

func NewRound(index uint64, merkle common.MerkleNodeID, root Transaction) Round {
	r := Round{
		idx:    index,
		merkle: merkle,
		root:   root,
	}

	r.id = blake2b.Sum256(r.Marshal())

	return r
}
