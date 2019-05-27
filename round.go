package wavelet

import (
	"bytes"
	"encoding/binary"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"io"
	"math"
)

// Round represents a network-wide finalized non-overlapping graph depth interval that
// is denoted by both a critical starting point transaction, and a critical ending point
// transaction. They contain the expected Merkle root of the ledgers state. They are
// denoted by either their index or ID, which is the checksum of applying BLAKE2b over
// its contents.
type Round struct {
	ID     RoundID
	Index  uint64
	Merkle MerkleNodeID

	Applied uint64

	Start Transaction
	End   Transaction
}

func NewRound(index uint64, merkle MerkleNodeID, applied uint64, start, end Transaction) Round {
	r := Round{
		Index:   index,
		Merkle:  merkle,
		Applied: applied,
		Start:   start,
		End:     end,
	}

	r.ID = blake2b.Sum256(r.Marshal())

	return r
}

func (r Round) Marshal() []byte {
	var w bytes.Buffer

	var buf [8]byte

	binary.BigEndian.PutUint64(buf[:], r.Index)
	w.Write(buf[:8])

	w.Write(r.Merkle[:])

	binary.BigEndian.PutUint64(buf[:], r.Applied)
	w.Write(buf[:8])

	w.Write(r.Start.Marshal())
	w.Write(r.End.Marshal())

	return w.Bytes()
}

func (r Round) ExpectedDifficulty(min byte, scale float64) byte {
	if r.End.Depth == 0 || r.Applied == 0 {
		return min
	}

	maxs := r.Applied
	mins := r.End.Depth - r.Start.Depth

	if mins > maxs {
		maxs, mins = mins, maxs
	}

	return byte(float64(min) + scale*math.Log2(float64(maxs)/float64(mins)))
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

	if _, err = io.ReadFull(r, buf[:]); err != nil {
		err = errors.Wrap(err, "failed to decode round num ancestors")
		return
	}

	round.Applied = binary.BigEndian.Uint64(buf[:8])

	if round.Start, err = UnmarshalTransaction(r); err != nil {
		err = errors.Wrap(err, "failed to decode round start transaction")
		return
	}

	if round.End, err = UnmarshalTransaction(r); err != nil {
		err = errors.Wrap(err, "failed to decode round end transaction")
		return
	}

	round.ID = blake2b.Sum256(round.Marshal())

	return
}
