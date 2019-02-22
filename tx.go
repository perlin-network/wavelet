package wavelet

import (
	"bytes"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/payload"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

const PublicKeySize = 32
const SignatureSize = 64
const MaxTransactionPayloadSize = 1024 * 100

var _ noise.Message = (*Transaction)(nil)

type Transaction struct {
	// WIRE FORMAT
	id [blake2b.Size256]byte

	sender, creator [PublicKeySize]byte

	parentIDs [][blake2b.Size256]byte

	timestamp uint64

	tag byte

	payload []byte

	senderSignature, creatorSignature [SignatureSize]byte

	// IN-MEMORY DATA
	children [][blake2b.Size256]byte
	depth    uint64
}

func (t *Transaction) IsCritical(difficulty int) bool {
	var buf bytes.Buffer
	_, _ = buf.Write(t.sender[:])

	for _, parentID := range t.parentIDs {
		_, _ = buf.Write(parentID[:])
	}

	checksum := blake2b.Sum256(buf.Bytes())

	return prefixLen(checksum[:]) >= difficulty
}

func (t *Transaction) Read(reader payload.Reader) (noise.Message, error) {
	n, err := reader.Read(t.sender[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode transaction sender")
	}

	if n != PublicKeySize {
		return nil, errors.New("could not read enough bytes for transaction sender")
	}

	n, err = reader.Read(t.creator[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode transaction creator")
	}

	if n != PublicKeySize {
		return nil, errors.New("could not read enough bytes for transaction creator")
	}

	numParents, err := reader.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read num parents")
	}

	for i := 0; i < int(numParents); i++ {
		var parentID [PublicKeySize]byte

		n, err = reader.Read(parentID[:])
		if err != nil {
			return nil, errors.Wrapf(err, "failed to decode parent %d", i)
		}

		if n != PublicKeySize {
			return nil, errors.Errorf("could not read enough bytes for parent %d", i)
		}

		t.parentIDs = append(t.parentIDs, parentID)
	}

	t.timestamp, err = reader.ReadUint64()
	if err != nil {
		return nil, errors.Wrap(err, "could not read transaction timestamp")
	}

	t.tag, err = reader.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "could not read transaction tag")
	}

	t.payload, err = reader.ReadBytes()
	if err != nil {
		return nil, errors.Wrap(err, "could not read transaction payload")
	}

	if len(t.payload) > MaxTransactionPayloadSize {
		return nil, errors.Errorf("transaction payload is of size %d, but can at most only handle %d bytes", len(t.payload), MaxTransactionPayloadSize)
	}

	n, err = reader.Read(t.senderSignature[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode sender signature")
	}

	if n != PublicKeySize {
		return nil, errors.New("could not read enough bytes for sender signature")
	}

	n, err = reader.Read(t.creatorSignature[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to decode creator signature")
	}

	if n != PublicKeySize {
		return nil, errors.New("could not read enough bytes for creator signature")
	}

	// TODO(kenta): have payload.Reader expose underlying byte buffer to not have to rewrite all bytes into a buffer and hash
	t.id = blake2b.Sum256(t.Write())

	return t, nil
}

func (t *Transaction) Write() []byte {
	writer := payload.NewWriter(nil)

	_, _ = writer.Write(t.sender[:])
	_, _ = writer.Write(t.creator[:])

	writer.WriteByte(byte(len(t.parentIDs)))

	for _, parentID := range t.parentIDs {
		_, _ = writer.Write(parentID[:])
	}

	writer.WriteUint64(t.timestamp)
	writer.WriteByte(t.tag)
	writer.WriteBytes(t.payload)

	_, _ = writer.Write(t.senderSignature[:])
	_, _ = writer.Write(t.creatorSignature[:])

	return writer.Bytes()
}
