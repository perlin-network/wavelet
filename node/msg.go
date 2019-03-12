package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/payload"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

var (
	_ noise.Message = (*GossipRequest)(nil)
	_ noise.Message = (*GossipResponse)(nil)
	_ noise.Message = (*QueryRequest)(nil)
	_ noise.Message = (*QueryResponse)(nil)
	_ noise.Message = (*SyncViewRequest)(nil)
	_ noise.Message = (*SyncViewResponse)(nil)
	_ noise.Message = (*SyncDiffMetadataRequest)(nil)
	_ noise.Message = (*SyncDiffMetadataResponse)(nil)
	_ noise.Message = (*SyncDiffChunkRequest)(nil)
	_ noise.Message = (*SyncDiffChunkResponse)(nil)
	_ noise.Message = (*SyncTransactionRequest)(nil)
	_ noise.Message = (*SyncTransactionResponse)(nil)
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
	if q.tx != nil {
		return q.tx.Write()
	}

	return nil
}

type QueryResponse struct {
	preferred *wavelet.Transaction
}

func (q QueryResponse) Read(reader payload.Reader) (noise.Message, error) {
	if reader.Len() > 0 {
		msg, err := wavelet.Transaction{}.Read(reader)
		if err != nil {
			return nil, errors.Wrap(err, "wavelet: failed to read query response preferred tx")
		}

		preferred := msg.(wavelet.Transaction)
		q.preferred = &preferred
	}

	return q, nil
}

func (q QueryResponse) Write() []byte {
	if q.preferred != nil {
		return q.preferred.Write()
	}

	return nil
}

type GossipRequest struct {
	tx *wavelet.Transaction
}

func (q GossipRequest) Read(reader payload.Reader) (noise.Message, error) {
	msg, err := wavelet.Transaction{}.Read(reader)
	if err != nil {
		return nil, errors.Wrap(err, "wavelet: failed to read gossip request tx")
	}

	tx := msg.(wavelet.Transaction)
	q.tx = &tx

	return q, nil
}

func (q GossipRequest) Write() []byte {
	if q.tx != nil {
		return q.tx.Write()
	}

	return nil
}

type GossipResponse struct {
	vote bool
}

func (q GossipResponse) Read(reader payload.Reader) (noise.Message, error) {
	vote, err := reader.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "wavelet: failed to read gossip response vote")
	}

	if vote == 1 {
		q.vote = true
	}

	return q, nil
}

func (q GossipResponse) Write() []byte {
	writer := payload.NewWriter(nil)

	if q.vote {
		writer.WriteByte(1)
	} else {
		writer.WriteByte(0)
	}

	return writer.Bytes()
}

type SyncViewRequest struct {
	root *wavelet.Transaction
}

func (s SyncViewRequest) Read(reader payload.Reader) (noise.Message, error) {
	msg, err := wavelet.Transaction{}.Read(reader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read root tx")
	}

	root := msg.(wavelet.Transaction)
	s.root = &root

	return s, nil
}

func (s SyncViewRequest) Write() []byte {
	if s.root != nil {
		return s.root.Write()
	}

	return nil
}

type SyncViewResponse struct {
	root *wavelet.Transaction
}

func (s SyncViewResponse) Read(reader payload.Reader) (noise.Message, error) {
	if reader.Len() > 0 {
		msg, err := wavelet.Transaction{}.Read(reader)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read root tx")
		}

		root := msg.(wavelet.Transaction)
		s.root = &root
	}

	return s, nil
}

func (s SyncViewResponse) Write() []byte {
	if s.root != nil {
		return s.root.Write()
	}

	return nil
}

type SyncDiffMetadataRequest struct {
	viewID uint64
}

type SyncDiffMetadataResponse struct {
	latestViewID uint64
	chunkHashes  [][blake2b.Size256]byte
}

type SyncDiffChunkRequest struct {
	chunkHash [blake2b.Size256]byte
}

type SyncDiffChunkResponse struct {
	found bool
	diff  []byte
}

func (s SyncDiffMetadataRequest) Read(reader payload.Reader) (noise.Message, error) {
	var err error

	s.viewID, err = reader.ReadUint64()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read view ID")
	}

	return s, nil
}

func (s SyncDiffMetadataRequest) Write() []byte {
	return payload.NewWriter(nil).WriteUint64(s.viewID).Bytes()
}

func (s SyncDiffMetadataResponse) Read(reader payload.Reader) (noise.Message, error) {
	var err error

	s.latestViewID, err = reader.ReadUint64()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read latest view id")
	}

	numChunks, err := reader.ReadUint32()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read num chunks")
	}

	for i := uint32(0); i < numChunks; i++ {
		var chunkHash [blake2b.Size256]byte

		n, err := reader.Read(chunkHash[:])

		if err != nil {
			return nil, errors.Wrap(err, "failed to read chunk hash")
		}

		if n != blake2b.Size256 {
			return nil, errors.New("did not read enough bytes for chunk hash")
		}

		s.chunkHashes = append(s.chunkHashes, chunkHash)
	}

	return s, nil
}

func (s SyncDiffMetadataResponse) Write() []byte {
	writer := payload.NewWriter(nil)

	writer.WriteUint64(s.latestViewID)
	writer.WriteUint32(uint32(len(s.chunkHashes)))

	for _, h := range s.chunkHashes {
		_, _ = writer.Write(h[:])
	}

	return writer.Bytes()
}

func (s SyncDiffChunkRequest) Read(reader payload.Reader) (noise.Message, error) {
	n, err := reader.Read(s.chunkHash[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to read chunk hash")
	}

	if n != blake2b.Size256 {
		return nil, errors.New("did not read enough bytes for chunk hash")
	}

	return s, nil
}

func (s SyncDiffChunkRequest) Write() []byte {
	return s.chunkHash[:]
}

func (s SyncDiffChunkResponse) Read(reader payload.Reader) (noise.Message, error) {
	var err error

	found, err := reader.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read found flag")
	}

	if found != 0 {
		s.found = true
	} else {
		s.found = false
	}

	s.diff, err = reader.ReadBytes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read hash")
	}

	return s, nil
}

func (s SyncDiffChunkResponse) Write() []byte {
	var found byte

	if s.found {
		found = 1
	}

	return payload.NewWriter(nil).WriteByte(found).WriteBytes(s.diff).Bytes()
}

type SyncTransactionResponse struct {
	tx *wavelet.Transaction
}

func (s SyncTransactionResponse) Read(reader payload.Reader) (noise.Message, error) {
	if reader.Len() > 0 {
		msg, err := wavelet.Transaction{}.Read(reader)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read root tx")
		}

		root := msg.(wavelet.Transaction)
		s.tx = &root
	}

	return s, nil
}

func (s SyncTransactionResponse) Write() []byte {
	if s.tx != nil {
		return s.tx.Write()
	}

	return nil
}

type SyncTransactionRequest struct {
	id common.TransactionID
}

func (s SyncTransactionRequest) Read(reader payload.Reader) (noise.Message, error) {
	id, err := reader.ReadBytes()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read id")
	}
	if len(id) != len(common.TransactionID{}) {
		return nil, errors.New("invalid transaction id")
	}
	copy(s.id[:], id)
	return s, nil
}

func (s SyncTransactionRequest) Write() []byte {
	return payload.NewWriter(nil).WriteBytes(s.id[:]).Bytes()
}
