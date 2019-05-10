package wavelet

import (
	"encoding/binary"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"io"
	"math"
)

// Nodes negotiate over which round to accept through Snowball. A round comprises of
// a Merkle root of the ledgers state proposed by the root transaction, alongside a
// single root transaction. Rounds are denoted by their ID, which is represented by
// BLAKE2b(merkle || root transactions content).
type Round struct {
	ID     common.RoundID
	Index  uint64
	Merkle common.MerkleNodeID

	Start Transaction
	End   Transaction
}

func (r Round) Marshal() []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], r.Index)

	return append(buf[:], append(r.Merkle[:], append(r.Start.Marshal(), r.End.Marshal()...)...)...)
}

func (r Round) ExpectedDifficulty(min byte, scale uint64) byte {
	if r.End.Depth == 0 && r.End.Confidence == 0 {
		return min
	}

	difficulty := byte(float64(min) + math.Log2(float64(r.End.Confidence-r.Start.Confidence)*float64(scale))/math.Log2(float64(r.End.Depth-r.Start.Depth)))

	//difficulty := byte(float64(min) + math.Exp(math.Log2(float64(r.End.Confidence - r.Start.Confidence)) / math.Log2(float64(r.End.Depth - r.Start.Depth))))
	if difficulty < min {
		difficulty = min
	}

	return difficulty
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

func NewRound(index uint64, merkle common.MerkleNodeID, start, end Transaction) Round {
	r := Round{
		Index:  index,
		Merkle: merkle,
		Start:  start,
		End:    end,
	}

	r.ID = blake2b.Sum256(r.Marshal())

	return r
}
