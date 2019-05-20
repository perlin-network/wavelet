package node

import (
	"bytes"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/pb/waveletpb"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"io"
	"io/ioutil"
)

type Transactions struct {
	txs []wavelet.Transaction
}

func (q Transactions) Marshal() ([]byte, error) {
	txBytes := make([][]byte, len(q.txs))
	for i, tx := range q.txs {
		txBytes[i] = tx.Marshal()
	}

	txs := &waveletpb.Transactions{Transactions: txBytes}
	return txs.Marshal()
}

func UnmarshalTransactions(r io.Reader) (Transactions, error) {
	var transactions Transactions
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return transactions, errors.Wrap(err, "failed to read data")
	}

	txs := &waveletpb.Transactions{}
	if err := txs.Unmarshal(data); err != nil {
		return transactions, errors.Wrap(err, "failed to unmarshal transactions from proto")
	}

	transactions.txs = make([]wavelet.Transaction, len(txs.Transactions))
	for i, tx := range txs.Transactions {
		if transactions.txs[i], err = wavelet.UnmarshalTransaction(bytes.NewReader(tx)); err != nil {
			return transactions, errors.Wrap(err, "failed to unmarshal transaction")
		}
	}

	return transactions, nil
}

type QueryRequest struct {
	round wavelet.Round
}

func (q QueryRequest) Marshal() ([]byte, error) {
	qr := &waveletpb.QueryRequest{Round: q.round.Marshal()}
	return qr.Marshal()
}

func UnmarshalQueryRequest(r io.Reader) (QueryRequest, error) {
	var queryRequest QueryRequest
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return queryRequest, errors.Wrap(err, "failed to read data")
	}

	qr := &waveletpb.QueryRequest{}
	if err := qr.Unmarshal(data); err != nil {
		return queryRequest, errors.Wrap(err, "failed to unmarshal query request from proto")
	}

	queryRequest.round, err = wavelet.UnmarshalRound(bytes.NewReader(qr.Round))
	if err != nil {
		return queryRequest, errors.Wrap(err, "failed to unmarshal round")
	}

	return queryRequest, nil
}

type QueryResponse struct {
	preferred wavelet.Round
}

func (q QueryResponse) Marshal() ([]byte, error) {
	qr := &waveletpb.QueryResponse{Preferred: q.preferred.Marshal()}
	return qr.Marshal()
}

func UnmarshalQueryResponse(r io.Reader) (QueryResponse, error) {
	var queryResponse QueryResponse
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return queryResponse, errors.Wrap(err, "failed to read data")
	}

	qr := &waveletpb.QueryResponse{}
	if err := qr.Unmarshal(data); err != nil {
		return queryResponse, errors.Wrap(err, "failed to unmarshal query response from proto")
	}

	queryResponse.preferred, err = wavelet.UnmarshalRound(bytes.NewReader(qr.Preferred))
	if err != nil {
		return queryResponse, errors.Wrap(err, "failed to unmarshal round")
	}

	return queryResponse, nil
}

type OutOfSyncResponse struct {
	round wavelet.Round
}

func (q OutOfSyncResponse) Marshal() ([]byte, error) {
	oosr := &waveletpb.OutOfSyncResponse{Round: q.round.Marshal()}
	return oosr.Marshal()
}

func UnmarshalOutOfSyncResponse(r io.Reader) (OutOfSyncResponse, error) {
	var outOfSyncResponse OutOfSyncResponse
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return outOfSyncResponse, errors.Wrap(err, "failed to read data")
	}

	oosr := &waveletpb.OutOfSyncResponse{}
	if err := oosr.Unmarshal(data); err != nil {
		return outOfSyncResponse, errors.Wrap(err, "failed to unmarshal out of sync response from proto")
	}

	outOfSyncResponse.round, err = wavelet.UnmarshalRound(bytes.NewReader(oosr.Round))
	if err != nil {
		return outOfSyncResponse, errors.Wrap(err, "failed to unmarshal round")
	}

	return outOfSyncResponse, nil
}

type SyncInitRequest struct {
	viewID uint64
}

func (s SyncInitRequest) Marshal() ([]byte, error) {
	sir := &waveletpb.SyncInitRequest{ViewId: s.viewID}
	return sir.Marshal()
}

func UnmarshalSyncInitRequest(r io.Reader) (SyncInitRequest, error) {
	var syncInitRequest SyncInitRequest
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return syncInitRequest, errors.Wrap(err, "failed to read data")
	}

	sir := &waveletpb.SyncInitRequest{}
	if err := sir.Unmarshal(data); err != nil {
		return syncInitRequest, errors.Wrap(err, "failed to unmarshal sync init request from proto")
	}

	syncInitRequest.viewID = sir.ViewId

	return syncInitRequest, nil
}

type SyncInitResponse struct {
	latestViewID uint64
	chunkHashes  [][blake2b.Size256]byte
}

func (s SyncInitResponse) Marshal() ([]byte, error) {
	sir := &waveletpb.SyncInitResponse{
		LatestViewId: s.latestViewID,
		ChunkHashes: make([][]byte, len(s.chunkHashes)),
	}

	for i, chunkHash := range s.chunkHashes {
		sir.ChunkHashes[i] = chunkHash[:]
	}

	return sir.Marshal()
}

func UnmarshalSyncInitResponse(r io.Reader) (SyncInitResponse, error) {
	var syncInitResponse SyncInitResponse
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return syncInitResponse, errors.Wrap(err, "failed to read data")
	}

	sir := &waveletpb.SyncInitResponse{}
	if err := sir.Unmarshal(data); err != nil {
		return syncInitResponse, errors.Wrap(err, "failed to unmarshal sync init response from proto")
	}

	syncInitResponse.latestViewID = sir.LatestViewId
	syncInitResponse.chunkHashes = make([][blake2b.Size256]byte, len(sir.ChunkHashes))
	for i, chunkHash := range sir.ChunkHashes {
		if _, err := io.ReadFull(bytes.NewReader(chunkHash), syncInitResponse.chunkHashes[i][:]); err != nil {
			return syncInitResponse, errors.Wrap(err, "failed read chunk hash from proto")
		}
	}

	return syncInitResponse, nil
}

type SyncChunkRequest struct {
	chunkHash [blake2b.Size256]byte
}

func (s SyncChunkRequest) Marshal() ([]byte, error) {
	scr := &waveletpb.SyncChunkRequest{ChunkHash: s.chunkHash[:]}
	return scr.Marshal()
}

func UnmarshalSyncChunkRequest(r io.Reader) (SyncChunkRequest, error) {
	var syncChunkRequest SyncChunkRequest
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return syncChunkRequest, errors.Wrap(err, "failed to read data")
	}

	sir := &waveletpb.SyncChunkRequest{}
	if err := sir.Unmarshal(data); err != nil {
		return syncChunkRequest, errors.Wrap(err, "failed to unmarshal sync chunk request from proto")
	}

	if _, err := io.ReadFull(bytes.NewReader(sir.ChunkHash), syncChunkRequest.chunkHash[:]); err != nil {
		return syncChunkRequest, errors.Wrap(err, "failed read chunk hash from proto")
	}

	return syncChunkRequest, nil
}

type SyncChunkResponse struct {
	diff []byte
}

func (s SyncChunkResponse) Marshal() ([]byte, error) {
	scr := &waveletpb.SyncChunkResponse{Diff: s.diff}
	return scr.Marshal()
}

func UnmarshalSyncChunkResponse(r io.Reader) (SyncChunkResponse, error) {
	var syncChunkResponse SyncChunkResponse
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return syncChunkResponse, errors.Wrap(err, "failed to read data")
	}

	scr := &waveletpb.SyncChunkResponse{}
	if err := scr.Unmarshal(data); err != nil {
		return syncChunkResponse, errors.Wrap(err, "failed to unmarshal sync chunk response from proto")
	}

	syncChunkResponse.diff = scr.Diff
	return syncChunkResponse, nil
}

type DownloadTxRequest struct {
	ids []common.TransactionID
}

func (s DownloadTxRequest) Marshal() ([]byte, error) {
	dtr := &waveletpb.DownloadTxRequest{
		Ids: make([][]byte, len(s.ids)),
	}

	for i, id := range s.ids {
		dtr.Ids[i] = id[:]
	}

	return dtr.Marshal()
}

func UnmarshalDownloadTxRequest(r io.Reader) (DownloadTxRequest, error) {
	var downloadTxRequest DownloadTxRequest
	data, err := ioutil.ReadAll(r)
	if err != nil {
		return downloadTxRequest, errors.Wrap(err, "failed to read data")
	}

	dtr := &waveletpb.DownloadTxRequest{}
	if err := dtr.Unmarshal(data); err != nil {
		return downloadTxRequest, errors.Wrap(err, "failed to unmarshal download tx request from proto")
	}

	downloadTxRequest.ids = make([]common.TransactionID, len(dtr.Ids))
	for i, id := range dtr.Ids {
		if _, err := io.ReadFull(bytes.NewReader(id), downloadTxRequest.ids[i][:]); err != nil {
			return downloadTxRequest, errors.Wrap(err, "failed read chunk hash from proto")
		}
	}

	return downloadTxRequest, nil
}
