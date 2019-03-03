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
	_ noise.Message = (*SyncRequest)(nil)
	_ noise.Message = (*SyncResponse)(nil)
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

	return writer.Bytes()
}

type SyncRequest struct {
	viewID uint64
}

func (s SyncRequest) Read(reader payload.Reader) (noise.Message, error) {
	var err error

	s.viewID, err = reader.ReadUint64()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read view ID")
	}

	return s, nil
}

func (s SyncRequest) Write() []byte {
	return payload.NewWriter(nil).WriteUint64(s.viewID).Bytes()
}

type SyncResponse struct {
	root   wavelet.Transaction
	viewID uint64
	diff   []byte
}

func (s SyncResponse) Read(reader payload.Reader) (noise.Message, error) {
	msg, err := wavelet.Transaction{}.Read(reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read root tx")
	}

	viewID, err := reader.ReadUint64()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read view id")
	}

	diff, err := reader.ReadBytes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read merkle tree diff")
	}

	s.root = msg.(wavelet.Transaction)
	s.viewID = viewID
	s.diff = diff

	return s, nil
}

func (s SyncResponse) Write() []byte {
	return payload.NewWriter(s.root.Write()).WriteUint64(s.viewID).WriteBytes(s.diff).Bytes()
}
