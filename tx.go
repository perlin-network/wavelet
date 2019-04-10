package wavelet

import (
	"bytes"
	"encoding/binary"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"io"
	"math/bits"
)

type Transaction struct {
	// WIRE FORMAT
	ID common.TransactionID

	Sender, Creator common.AccountID

	ParentIDs []common.TransactionID

	Timestamp uint64

	ViewID uint64

	Tag byte

	Payload []byte

	// Only set if the transaction is a critical transaction.
	AccountsMerkleRoot common.MerkleNodeID

	// Only set if the transaction is a critical transaction.
	DifficultyTimestamps []uint64

	SenderSignature, CreatorSignature common.Signature

	// IN-MEMORY DATA
	depth uint64
}

func prefixLen(buf []byte) int {
	for i, b := range buf {
		if b != 0 {
			return i*8 + bits.LeadingZeros8(uint8(b))
		}
	}

	return len(buf)*8 - 1
}

func (t Transaction) getCriticalSeed() [blake2b.Size256]byte {
	var buf bytes.Buffer
	_, _ = buf.Write(t.Sender[:])

	for _, parentID := range t.ParentIDs {
		_, _ = buf.Write(parentID[:])
	}

	return blake2b.Sum256(buf.Bytes())
}

func (t Transaction) IsCritical(difficulty uint64) bool {
	checksum := t.getCriticalSeed()

	return prefixLen(checksum[:]) >= int(difficulty)
}

func (t Transaction) Marshal() []byte {
	var w bytes.Buffer

	_, _ = w.Write(t.Sender[:])
	_, _ = w.Write(t.Creator[:])

	_ = w.WriteByte(byte(len(t.ParentIDs)))

	for _, parentID := range t.ParentIDs {
		_, _ = w.Write(parentID[:])
	}

	var buf [8]byte

	binary.BigEndian.PutUint64(buf[:], t.Timestamp)
	_, _ = w.Write(buf[:])

	binary.BigEndian.PutUint64(buf[:], t.ViewID)
	_, _ = w.Write(buf[:])

	_ = w.WriteByte(t.Tag)

	binary.BigEndian.PutUint32(buf[:4], uint32(len(t.Payload)))
	_, _ = w.Write(buf[:4])

	_, _ = w.Write(t.Payload)

	critical := t.AccountsMerkleRoot != common.ZeroMerkleNodeID

	if critical {
		_ = w.WriteByte(1)
		_, _ = w.Write(t.AccountsMerkleRoot[:])

		_ = w.WriteByte(byte(len(t.DifficultyTimestamps)))

		for _, timestamp := range t.DifficultyTimestamps {
			binary.BigEndian.PutUint64(buf[:], timestamp)
			_, _ = w.Write(buf[:])
		}
	} else {
		_ = w.WriteByte(0)
	}

	_, _ = w.Write(t.SenderSignature[:])
	_, _ = w.Write(t.CreatorSignature[:])

	return w.Bytes()
}

func UnmarshalTransaction(r io.Reader) (t Transaction, err error) {
	if _, err = io.ReadFull(r, t.Sender[:]); err != nil {
		err = errors.Wrap(err, "failed to decode transaction sender")
		return
	}

	if _, err = io.ReadFull(r, t.Creator[:]); err != nil {
		err = errors.Wrap(err, "failed to decode transaction creator")
		return
	}

	var buf [8]byte

	if _, err = io.ReadFull(r, buf[:1]); err != nil {
		err = errors.Wrap(err, "failed to read num parents")
		return
	}

	t.ParentIDs = make([]common.TransactionID, buf[0])

	for i := range t.ParentIDs {
		if _, err = io.ReadFull(r, t.ParentIDs[i][:]); err != nil {
			err = errors.Wrapf(err, "failed to decode parent %d", i)
			return
		}
	}

	if _, err = io.ReadFull(r, buf[:]); err != nil {
		err = errors.Wrap(err, "could not read transaction timestamp")
		return
	}

	t.Timestamp = binary.BigEndian.Uint64(buf[:])

	if _, err = io.ReadFull(r, buf[:]); err != nil {
		err = errors.Wrap(err, "could not read transaction view ID")
		return
	}

	t.ViewID = binary.BigEndian.Uint64(buf[:])

	if _, err = io.ReadFull(r, buf[:1]); err != nil {
		err = errors.Wrap(err, "could not read transaction tag")
		return
	}

	t.Tag = buf[0]

	if _, err = io.ReadFull(r, buf[:4]); err != nil {
		err = errors.Wrap(err, "could not read transaction payload length")
		return
	}

	t.Payload = make([]byte, binary.BigEndian.Uint32(buf[:4]))

	if _, err = io.ReadFull(r, t.Payload[:]); err != nil {
		err = errors.Wrap(err, "could not read transaction payload")
		return
	}

	if _, err = io.ReadFull(r, buf[:1]); err != nil {
		err = errors.Wrap(err, "could not read checking bit to see if tx is critical")
		return
	}

	critical := buf[0]

	if critical == 1 {
		if _, err = io.ReadFull(r, t.AccountsMerkleRoot[:]); err != nil {
			err = errors.Wrap(err, "failed to decode accounts merkle root")
			return
		}

		if _, err = io.ReadFull(r, buf[:1]); err != nil {
			err = errors.Wrap(err, "could not read number of difficulty timestamps")
			return
		}

		if int(buf[0]) > sys.CriticalTimestampAverageWindowSize {
			err = errors.Errorf("got %d difficulty timestamps when only expected %d timestamps", int(buf[0]), sys.CriticalTimestampAverageWindowSize)
			return
		}

		t.DifficultyTimestamps = make([]uint64, buf[0])

		for i := range t.DifficultyTimestamps {
			if _, err = io.ReadFull(r, buf[:]); err != nil {
				err = errors.Wrap(err, "could not read difficulty timestamp")
				return
			}

			t.DifficultyTimestamps[i] = binary.BigEndian.Uint64(buf[:])
		}
	}

	if _, err = io.ReadFull(r, t.SenderSignature[:]); err != nil {
		err = errors.Wrap(err, "failed to decode sender signature")
		return
	}

	if _, err = io.ReadFull(r, t.CreatorSignature[:]); err != nil {
		err = errors.Wrap(err, "failed to decode creator signature")
		return
	}

	t.rehash()

	return t, nil
}

func (t *Transaction) rehash() *Transaction {
	t.ID = blake2b.Sum256(t.Marshal())
	return t
}

func (t *Transaction) Depth() uint64 {
	return t.depth
}
