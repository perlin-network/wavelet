package wavelet

import (
	"bytes"
	"context"
	"fmt"
	"github.com/pkg/errors"
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

func (p *Protocol) InitializeSync(context.Context, *SyncRequest) (*SyncResponse, error) {
	return nil, nil
}

func (p *Protocol) CheckOutOfSync(context.Context, *OutOfSyncRequest) (*OutOfSyncResponse, error) {
	return nil, nil
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

func (p *Protocol) DownloadChunk(context.Context, *DownloadChunkRequest) (*DownloadChunkResponse, error) {
	return nil, nil
}
