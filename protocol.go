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
	"io"

	"github.com/perlin-network/wavelet/conf"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/willf/bloom"
	"golang.org/x/crypto/blake2b"
)

type Protocol struct {
	ledger *Ledger
}

func (p *Protocol) Query(ctx context.Context, req *QueryRequest) (*QueryResponse, error) {
	res := &QueryResponse{}

	block, err := p.ledger.blocks.GetByIndex(req.BlockIndex)
	if err == nil {
		res.Block = block.Marshal()
		return res, nil
	}

	preferred := p.ledger.finalizer.Preferred()
	if preferred != nil {
		res.Block = preferred.(*Block).Marshal()
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
	header := &SyncInfo{LatestRound: p.ledger.rounds.Latest().Marshal()}

	diffBuffer := p.ledger.fileBuffers.GetUnbounded()
	defer p.ledger.fileBuffers.Put(diffBuffer)

	if err := p.ledger.accounts.Snapshot().DumpDiff(req.GetRoundId(), diffBuffer); err != nil {
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
		n, err := chunksBuffer.ReadAt(chunkBuf[:], int64(i*syncChunkSize))
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
		OutOfSync: p.ledger.rounds.Latest().Index >= conf.GetSyncIfRoundsDifferBy()+req.RoundIndex,
	}, nil
}

func (p *Protocol) DownloadMissingTx(ctx context.Context, req *DownloadMissingTxRequest) (*DownloadMissingTxResponse, error) {
	res := &DownloadMissingTxResponse{
		Transactions: [][]byte{},
	}

	existing := bloom.New(32, 2) // m and k values here will be replaced by ReadFrom
	if _, err := existing.ReadFrom(bytes.NewReader(req.TransactionIds)); err != nil {
		return nil, err
	}

	missingIDs := make([]TransactionID, 0)
	p.ledger.mempool.Ascend(func(txID TransactionID) bool {
		// TODO: limit no. of transactions per request
		// Add tx not in the bloom filter
		if !existing.Test(txID[:]) {
			missingIDs = append(missingIDs, txID)
		}

		return true
	})

	missingTxs := p.ledger.mempool.Resolve(missingIDs)
	for _, tx := range missingTxs {
		res.Transactions = append(res.Transactions, tx.Marshal())
	}

	logger := log.Sync("pull_tx")
	logger.Info().
		Int("missing_txs", len(res.Transactions)).
		Msg("Responded to download missing tx request.")

	return res, nil
}
