// Copyright (c) 2019 Perlin
//
// Permission is hereby granted, free of charge, to any person obtaining a copy of
// this software and associated documentation files (the "Software"), to deal in
// the Software without restriction, including without limitation the rights to
// use, copy, modify, merge, publish, distribute, sublicense, and/or sell copies of
// the Software, and to permit persons to whom the Software is furnished to do so,
// subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY, FITNESS
// FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE AUTHORS OR
// COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER LIABILITY, WHETHER
// IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM, OUT OF OR IN
// CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE SOFTWARE.

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

	Start *Transaction
	End   *Transaction
}

func NewRound(index uint64, merkle MerkleNodeID, applied uint64, start, end *Transaction) Round {
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
