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
	"context"
	"github.com/perlin-network/wavelet/internal/cuckoo"
	"io"

	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"golang.org/x/crypto/blake2b"
)

type Protocol struct {
	ledger *Ledger
}

func (p *Protocol) Query(ctx context.Context, req *QueryRequest) (*QueryResponse, error) {
	res := &QueryResponse{}

	latestBlock := p.ledger.blocks.Latest()

	var (
		block *Block
		err   error
	)

	// Return preferred block if peer is finalizing on the same block
	if latestBlock.Index+1 == req.BlockIndex {
		preferred := p.ledger.finalizer.Preferred()
		if preferred != nil {
			block = preferred.Value().(*Block)
		}
	}

	// Otherwise, return the finalized block
	if latestBlock.Index+1 > req.BlockIndex {
		block, err = p.ledger.blocks.GetByIndex(req.BlockIndex)
		if err != nil {
			return nil, err
		}
	}

	if block == nil {
		return res, nil
	}

	// Check cache block ID
	if req.CacheBlockId != nil {
		if bytes.Equal(block.ID[:], req.CacheBlockId) {
			res.CacheValid = true
			return res, nil
		}
	}

	payload, err := block.Marshal()
	if err != nil {
		return nil, err
	}

	res.Block = payload

	return res, nil
}

func (p *Protocol) Sync(stream Wavelet_SyncServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}

	res := &SyncResponse{}

	block, err := p.ledger.blocks.Latest().Marshal()
	if err != nil {
		return err
	}

	header := &SyncInfo{Block: block}
	diffBuffer := p.ledger.fileBuffers.GetUnbounded()

	defer p.ledger.fileBuffers.Put(diffBuffer)

	if err := p.ledger.accounts.Snapshot().DumpDiff(req.GetBlockId(), diffBuffer); err != nil {
		return err
	}

	chunksBuffer, err := p.ledger.fileBuffers.GetBounded(diffBuffer.Len())
	if err != nil {
		return err
	}

	defer p.ledger.fileBuffers.Put(chunksBuffer)

	if _, err := io.Copy(chunksBuffer, diffBuffer); err != nil {
		return err
	}

	type chunkInfo struct {
		Idx  int
		Size int
	}

	chunks := map[[blake2b.Size256]byte]chunkInfo{}

	// Chunk dumped diff
	syncChunkSize := conf.GetSyncChunkSize()
	chunkBuf := make([]byte, syncChunkSize)

	var i int

	for {
		n, err := chunksBuffer.ReadAt(chunkBuf, int64(i*syncChunkSize))
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, chunkBuf[:n])
			checksum := blake2b.Sum256(chunk)
			header.Checksums = append(header.Checksums, checksum[:])

			chunks[checksum] = chunkInfo{Idx: i, Size: n}
			i++
		}

		if err == io.EOF {
			break
		} else if err != nil {
			return err
		}
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

		info, ok := chunks[checksum]
		if !ok {
			res.Data.(*SyncResponse_Chunk).Chunk = nil

			if err = stream.Send(res); err != nil {
				return err
			}
		}

		if _, err := chunksBuffer.ReadAt(chunkBuf[:info.Size], int64(info.Idx*sys.SyncChunkSize)); err != nil {
			return err
		}

		logger := log.Sync("provide_chunk")
		logger.Info().
			Hex("requested_hash", req.GetChecksum()).
			Msg("Responded to sync chunk request.")

		res.Data.(*SyncResponse_Chunk).Chunk = chunkBuf[:info.Size]

		if err = stream.Send(res); err != nil {
			return err
		}
	}
}

func (p *Protocol) CheckOutOfSync(ctx context.Context, req *OutOfSyncRequest) (*OutOfSyncResponse, error) {
	return &OutOfSyncResponse{
		OutOfSync: p.ledger.blocks.Latest().Index >= conf.GetSyncIfBlockIndicesDifferBy()+req.BlockIndex,
	}, nil
}

func (p *Protocol) SyncTransactions(stream Wavelet_SyncTransactionsServer) error {
	req, err := stream.Recv()
	if err != nil {
		return err
	}

	cf, err := cuckoo.UnmarshalBinary(req.GetFilter())
	if err != nil {
		return err
	}

	var toReturn [][]byte

	p.ledger.transactions.Iterate(func(tx *Transaction) bool {
		if exists := cf.Lookup(tx.ID); !exists {
			toReturn = append(toReturn, tx.Marshal())
		}

		return true
	})

	res := &TransactionsSyncResponse{
		Data: &TransactionsSyncResponse_TransactionsNum{
			TransactionsNum: uint64(len(toReturn)),
		},
	}

	if err := stream.Send(res); err != nil {
		return err
	}

	pointer := 0

	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}

		chunkSize := int(req.GetChunkSize())
		if chunkSize > len(toReturn) {
			chunkSize = len(toReturn)
		}

		res := &TransactionsSyncResponse{
			Data: &TransactionsSyncResponse_Transactions{
				Transactions: &TransactionsSyncPart{
					Transactions: toReturn[pointer : pointer+chunkSize],
				},
			},
		}

		if err := stream.Send(res); err != nil {
			return err
		}

		pointer += chunkSize
		if pointer >= len(toReturn) {
			break
		}
	}

	if pointer > 0 {
		logger := log.Sync("sync_tx")
		logger.Debug().
			Int("num_transactions", len(toReturn)).
			Msg("Provided transactions for a sync request.")
	}

	return nil
}

func (p *Protocol) PullTransactions(
	ctx context.Context, req *TransactionPullRequest) (*TransactionPullResponse, error,
) {
	res := &TransactionPullResponse{
		Transactions: make([][]byte, 0, len(req.TransactionIds)),
	}

	var txID TransactionID

	for _, id := range req.TransactionIds {
		copy(txID[:], id)

		if tx := p.ledger.transactions.Find(txID); tx != nil {
			res.Transactions = append(res.Transactions, tx.Marshal())
		}
	}

	if len(res.Transactions) > 0 {
		logger := log.Sync("pull_missing_tx")
		logger.Debug().
			Int("num_transactions", len(res.Transactions)).
			Msg("Provided transactions for a pull request.")
	}

	return res, nil
}
