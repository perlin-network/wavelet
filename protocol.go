package wavelet

import (
	"bytes"
	"context"
	"fmt"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

type Protocol struct {
	ledger *Ledger
}

func (p *Protocol) Gossip(stream Wavelet_GossipServer) error {
	for {
		batch, err := stream.Recv()

		if err != nil {
			return err
		}

		for _, buf := range batch.Transactions {
			tx, err := UnmarshalTransaction(bytes.NewReader(buf))

			if err != nil {
				continue
			}

			if err := p.ledger.AddTransaction(tx); err != nil && errors.Cause(err) != ErrMissingParents {
				fmt.Printf("error adding incoming tx to graph [%v]: %+v\n", err, tx)
			}
		}
	}
}

func (p *Protocol) Query(ctx context.Context, req *QueryRequest) (*QueryResponse, error) {
	res := &QueryResponse{}

	round, err := p.ledger.rounds.GetByIndex(req.RoundIndex)

	if err == nil {
		res.Round = round.Marshal()
		return res, nil
	}

	preferred := p.ledger.finalizer.Preferred()

	if preferred != nil {
		res.Round = preferred.Marshal()
		return res, nil
	}

	return res, nil
}

func (p *Protocol) Sync(stream Wavelet_SyncServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}

	res := &SyncResponse{}

	diff := p.ledger.accounts.Snapshot().DumpDiff(req.GetRoundId())
	header := &SyncInfo{LatestRoundId: p.ledger.rounds.Count()}

	for i := 0; i < len(diff); i += sys.SyncChunkSize {
		end := i + sys.SyncChunkSize

		if end > len(diff) {
			end = len(diff)
		}

		checksum := blake2b.Sum256(diff[i:end])
		p.ledger.cacheChunks.put(checksum, diff[i:end])

		header.Checksums = append(header.Checksums, checksum[:])
	}

	res.Data = &SyncResponse_Header{Header: header}

	if err := stream.Send(res); err != nil {
		return err
	}

	res.Data = &SyncResponse_Chunk{}

	for {
		req, err := stream.Recv()

		if err != nil {
			return err
		}

		var checksum [blake2b.Size256]byte
		copy(checksum[:], req.GetChecksum())

		if chunk, found := p.ledger.cacheChunks.load(checksum); found {
			chunk := chunk.([]byte)

			logger := log.Sync("provide_chunk")
			logger.Info().
				Hex("requested_hash", req.GetChecksum()).
				Msg("Responded to sync chunk request.")

			res.Data.(*SyncResponse_Chunk).Chunk = chunk
		} else {
			res.Data.(*SyncResponse_Chunk).Chunk = nil
		}

		if err = stream.Send(res); err != nil {
			return err
		}
	}
}

func (p *Protocol) CheckOutOfSync(context.Context, *OutOfSyncRequest) (*OutOfSyncResponse, error) {
	return &OutOfSyncResponse{Round: p.ledger.rounds.Latest().Marshal()}, nil
}

func (p *Protocol) DownloadTx(ctx context.Context, req *DownloadTxRequest) (*DownloadTxResponse, error) {
	res := &DownloadTxResponse{Transactions: make([][]byte, 0, len(req.Ids))}

	for _, buf := range req.Ids {
		var id TransactionID
		copy(id[:], buf)

		if tx := p.ledger.Graph().FindTransaction(id); tx != nil {
			res.Transactions = append(res.Transactions, tx.Marshal())
		}
	}

	return res, nil
}
