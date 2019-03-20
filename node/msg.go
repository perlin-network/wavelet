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
	_ noise.Message = (*SyncInitRequest)(nil)
	_ noise.Message = (*SyncInitResponse)(nil)
	_ noise.Message = (*SyncChunkRequest)(nil)
	_ noise.Message = (*SyncChunkResponse)(nil)
	_ noise.Message = (*SyncMissingTxRequest)(nil)
	_ noise.Message = (*SyncMissingTxResponse)(nil)
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
	TX *wavelet.Transaction
}

func (q GossipRequest) Read(reader payload.Reader) (noise.Message, error) {
	msg, err := wavelet.Transaction{}.Read(reader)
	if err != nil {
		return nil, errors.Wrap(err, "wavelet: failed to read gossip request tx")
	}

	tx := msg.(wavelet.Transaction)
	q.TX = &tx

	return q, nil
}

func (q GossipRequest) Write() []byte {
	if q.TX != nil {
		return q.TX.Write()
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

type SyncInitRequest struct {
	viewID uint64
}

type SyncInitResponse struct {
	latestViewID uint64
	chunkHashes  [][blake2b.Size256]byte
}

type SyncChunkRequest struct {
	chunkHash [blake2b.Size256]byte
}

type SyncChunkResponse struct {
	found bool
	diff  []byte
}

func (s SyncInitRequest) Read(reader payload.Reader) (noise.Message, error) {
	var err error

	s.viewID, err = reader.ReadUint64()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read view ID")
	}

	return s, nil
}

func (s SyncInitRequest) Write() []byte {
	return payload.NewWriter(nil).WriteUint64(s.viewID).Bytes()
}

func (s SyncInitResponse) Read(reader payload.Reader) (noise.Message, error) {
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

func (s SyncInitResponse) Write() []byte {
	writer := payload.NewWriter(nil)

	writer.WriteUint64(s.latestViewID)
	writer.WriteUint32(uint32(len(s.chunkHashes)))

	for _, h := range s.chunkHashes {
		_, _ = writer.Write(h[:])
	}

	return writer.Bytes()
}

func (s SyncChunkRequest) Read(reader payload.Reader) (noise.Message, error) {
	n, err := reader.Read(s.chunkHash[:])
	if err != nil {
		return nil, errors.Wrap(err, "failed to read chunk hash")
	}

	if n != blake2b.Size256 {
		return nil, errors.New("did not read enough bytes for chunk hash")
	}

	return s, nil
}

func (s SyncChunkRequest) Write() []byte {
	return s.chunkHash[:]
}

func (s SyncChunkResponse) Read(reader payload.Reader) (noise.Message, error) {
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

func (s SyncChunkResponse) Write() []byte {
	var found byte

	if s.found {
		found = 1
	}

	return payload.NewWriter(nil).WriteByte(found).WriteBytes(s.diff).Bytes()
}

type SyncMissingTxResponse struct {
	transactions []wavelet.Transaction
}

func (s SyncMissingTxResponse) Read(reader payload.Reader) (noise.Message, error) {
	if reader.Len() > 0 {
		numTransactions, err := reader.ReadByte()
		if err != nil {
			return nil, errors.Wrap(err, "failed to read number of transactions in sync transaction response")
		}

		for i := byte(0); i < numTransactions; i++ {
			msg, err := wavelet.Transaction{}.Read(reader)
			if err != nil {
				return nil, errors.Wrap(err, "failed to read root tx")
			}

			s.transactions = append(s.transactions, msg.(wavelet.Transaction))
		}
	}

	return s, nil
}

func (s SyncMissingTxResponse) Write() []byte {
	writer := payload.NewWriter(nil)

	writer.WriteByte(byte(len(s.transactions)))

	for _, tx := range s.transactions {
		_, _ = writer.Write(tx.Write())
	}

	return writer.Bytes()
}

type SyncMissingTxRequest struct {
	ids []common.TransactionID
}

func (s SyncMissingTxRequest) Read(reader payload.Reader) (noise.Message, error) {
	numIDs, err := reader.ReadByte()
	if err != nil {
		return nil, errors.Wrap(err, "failed to read number of ids in sync transaction request")
	}

	for i := byte(0); i < numIDs; i++ {
		var id common.TransactionID

		n, err := reader.Read(id[:])

		if err != nil {
			return nil, errors.Wrap(err, "failed to read id")
		}

		if n != common.SizeTransactionID {
			return nil, errors.New("invalid transaction id")
		}

		s.ids = append(s.ids, id)
	}
	return s, nil
}

func (s SyncMissingTxRequest) Write() []byte {
	writer := payload.NewWriter(nil)

	writer.WriteByte(byte(len(s.ids)))

	for _, id := range s.ids {
		_, _ = writer.Write(id[:])
	}

	return writer.Bytes()
}
