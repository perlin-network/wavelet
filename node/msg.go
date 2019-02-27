package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

var (
	_ noise.Message = (*QueryRequest)(nil)
	_ noise.Message = (*QueryResponse)(nil)
)

type QueryRequest struct {
	tx *wavelet.Transaction
}

func (q QueryRequest) Read(reader payload.Reader) (noise.Message, error) {
	msg, err := wavelet.Transaction{}.Read(reader)
	if err != nil {
		return nil, errors.Wrap(err, "wavelet: failed to read query request tx")
	}

	tx := msg.(wavelet.Transaction)
	q.tx = &tx

	return q, nil
}

func (q QueryRequest) Write() []byte {
	return q.tx.Write()
}

type QueryResponse struct {
	id   [blake2b.Size256]byte
	vote bool

	signature [wavelet.SignatureSize]byte
}

func (q QueryResponse) Read(reader payload.Reader) (noise.Message, error) {
	n, err := reader.Read(q.id[:])

	if err != nil {
		return nil, errors.Wrap(err, "wavelet: failed to read query response id")
	}

	if n != len(q.id) {
		return nil, errors.New("wavelet: didn't read enough bytes for query response id")
	}

	vote, err := reader.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "wavelet: faile to read query response vote")
	}

	if vote == 1 {
		q.vote = true
	}

	n, err = reader.Read(q.signature[:])

	if err != nil {
		return nil, errors.Wrap(err, "wavelet: failed to read query response signature")
	}

	if n != len(q.signature) {
		return nil, errors.New("wavelet: didn't read enough bytes for query response signature")
	}

	return q, nil
}

func (q QueryResponse) Write() []byte {
	writer := payload.NewWriter(nil)
	_, _ = writer.Write(q.id[:])

	if q.vote {
		writer.WriteByte(1)
	} else {
		writer.WriteByte(0)
	}

	_, _ = writer.Write(q.signature[:])
	return writer.Bytes()
}
