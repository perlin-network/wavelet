package wavelet

import (
	"bytes"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/wavelet/avl"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/conflict"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"math/bits"
)

var _ noise.Message = (*Transaction)(nil)
var _ conflict.Item = (*Transaction)(nil)

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
	AccountsMerkleRoot [avl.MerkleHashSize]byte

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

func (t Transaction) Hash() interface{} {
	return t.ID
}

func (t Transaction) IsCritical(difficulty uint64) bool {
	var buf bytes.Buffer
	_, _ = buf.Write(t.Sender[:])

	for _, parentID := range t.ParentIDs {
		_, _ = buf.Write(parentID[:])
	}

	checksum := blake2b.Sum256(buf.Bytes())

	return prefixLen(checksum[:]) >= int(difficulty)
}

func (t Transaction) Read(reader payload.Reader) (noise.Message, error) {
	n, err := reader.Read(t.Sender[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode transaction sender")
	}

	if n != common.SizeAccountID {
		return nil, errors.New("could not read enough bytes for transaction sender")
	}

	n, err = reader.Read(t.Creator[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode transaction creator")
	}

	if n != common.SizeAccountID {
		return nil, errors.New("could not read enough bytes for transaction creator")
	}

	numParents, err := reader.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read num parents")
	}

	for i := byte(0); i < numParents; i++ {
		var parentID common.TransactionID

		n, err = reader.Read(parentID[:])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode parent %d", i)
		}

		if n != common.SizeAccountID {
			return nil, errors.Errorf("could not read enough bytes for parent %d", i)
		}

		t.ParentIDs = append(t.ParentIDs, parentID)
	}

	t.Timestamp, err = reader.ReadUint64()
	if err != nil {
		return nil, errors.Wrap(err, "could not read transaction timestamp")
	}

	t.ViewID, err = reader.ReadUint64()
	if err != nil {
		return nil, errors.Wrap(err, "could not read transaction view ID")
	}

	t.Tag, err = reader.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "could not read transaction tag")
	}

	t.Payload, err = reader.ReadBytes()
	if err != nil {
		return nil, errors.Wrap(err, "could not read transaction payload")
	}

	// If there exists an account merkle root, read it.
	if reader.Len() > common.SizeSignature*2 {
		n, err = reader.Read(t.AccountsMerkleRoot[:])
		if err != nil {
			return nil, errors.Wrap(err, "failed to decode accounts merkle root")
		}

		if n != avl.MerkleHashSize {
			return nil, errors.New("could not read enough bytes for accounts merkle root")
		}

		n, err := reader.ReadByte()
		if err != nil {
			return nil, errors.Wrap(err, "could not read number of difficulty timestamps")
		}

		if int(n) > sys.CriticalTimestampAverageWindowSize {
			return nil, errors.Errorf("got %d difficulty timestamps when only expected %d timestamps", int(n), sys.CriticalTimestampAverageWindowSize)
		}

		for i := 0; i < int(n); i++ {
			timestamp, err := reader.ReadUint64()
			if err != nil {
				return nil, errors.Wrap(err, "failed to read difficulty timestamp")
			}

			t.DifficultyTimestamps = append(t.DifficultyTimestamps, timestamp)
		}
	}

	n, err = reader.Read(t.SenderSignature[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode sender signature")
	}

	if n != common.SizeSignature {
		return nil, errors.New("could not read enough bytes for sender signature")
	}

	n, err = reader.Read(t.CreatorSignature[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode creator signature")
	}

	if n != common.SizeSignature {
		return nil, errors.New("could not read enough bytes for creator signature")
	}

	t.rehash()

	return t, nil
}

func (t Transaction) Write() []byte {
	writer := payload.NewWriter(nil)

	_, _ = writer.Write(t.Sender[:])
	_, _ = writer.Write(t.Creator[:])

	writer.WriteByte(byte(len(t.ParentIDs)))

	for _, parentID := range t.ParentIDs {
		_, _ = writer.Write(parentID[:])
	}

	writer.WriteUint64(t.Timestamp)
	writer.WriteUint64(t.ViewID)
	writer.WriteByte(t.Tag)
	writer.WriteBytes(t.Payload)

	var zero [avl.MerkleHashSize]byte

	if t.AccountsMerkleRoot != zero {
		_, _ = writer.Write(t.AccountsMerkleRoot[:])

		writer.WriteByte(byte(len(t.DifficultyTimestamps)))
		for _, timestamp := range t.DifficultyTimestamps {
			writer.WriteUint64(timestamp)
		}
	}

	_, _ = writer.Write(t.SenderSignature[:])
	_, _ = writer.Write(t.CreatorSignature[:])

	return writer.Bytes()
}

func (t *Transaction) rehash() *Transaction {
	t.ID = blake2b.Sum256(t.Write())
	return t
}

func (t *Transaction) Depth() uint64 {
	return t.depth
}
