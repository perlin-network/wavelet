package node

import (
	"encoding/binary"
	"github.com/perlin-network/wavelet"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"io"
)

type QueryRequest struct {
	round wavelet.Round
}

func (q QueryRequest) Marshal() []byte {
	return q.round.Marshal()
}

func UnmarshalQueryRequest(r io.Reader) (q QueryRequest, err error) {
	q.round, err = wavelet.UnmarshalRound(r)

	if err != nil {
		err = errors.Wrap(err, "failed to read query request")
		return
	}

	return
}

type QueryResponse struct {
	preferred wavelet.Round
}

func (q QueryResponse) Marshal() []byte {
	return q.preferred.Marshal()
}

func UnmarshalQueryResponse(r io.Reader) (q QueryResponse, err error) {
	q.preferred, err = wavelet.UnmarshalRound(r)

	if err != nil {
		err = errors.Wrap(err, "failed to read query response")
		return
	}

	return
}

type GossipRequest struct {
	tx wavelet.Transaction
}

func (q GossipRequest) Marshal() []byte {
	return q.tx.Marshal()
}

func UnmarshalGossipRequest(r io.Reader) (q GossipRequest, err error) {
	q.tx, err = wavelet.UnmarshalTransaction(r)

	if err != nil {
		err = errors.Wrap(err, "failed to read gossip request")
		return
	}

	return
}

type GossipResponse struct {
	vote bool
}

func (q GossipResponse) Marshal() []byte {
	var buf [1]byte

	if q.vote {
		buf[0] = 1
	} else {
		buf[0] = 0
	}

	return buf[:]
}

func UnmarshalGossipResponse(r io.Reader) (q GossipResponse, err error) {
	var buf [1]byte

	if _, err := io.ReadFull(r, buf[:]); err != nil {
		return q, errors.Wrap(err, "failed to read vote in gossip response")
	}

	if buf[0] == 1 {
		q.vote = true
	}

	return
}

type OutOfSyncResponse struct {
	round wavelet.Round
}

func (q OutOfSyncResponse) Marshal() []byte {
	return q.round.Marshal()
}

func UnmarshalOutOfSyncResponse(r io.Reader) (q OutOfSyncResponse, err error) {
	q.round, err = wavelet.UnmarshalRound(r)

	if err != nil {
		err = errors.Wrap(err, "failed to read round in sync view response")
		return
	}

	return
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
	diff []byte
}

type SyncMissingTxRequest struct {
	checksums []uint64
}

type SyncMissingTxResponse struct {
	transactions []wavelet.Transaction
}

func (s SyncInitRequest) Marshal() []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], s.viewID)

	return buf[:]
}

func UnmarshalSyncInitRequest(r io.Reader) (q SyncInitRequest, err error) {
	var buf [8]byte

	if _, err = io.ReadFull(r, buf[:]); err != nil {
		return
	}

	q.viewID = binary.BigEndian.Uint64(buf[:])

	return
}

func (s SyncInitResponse) Marshal() []byte {
	buf := make([]byte, 8+4+blake2b.Size256*len(s.chunkHashes))

	binary.BigEndian.PutUint64(buf[0:8], s.latestViewID)
	binary.BigEndian.PutUint32(buf[8:12], uint32(len(s.chunkHashes)))

	for i, chunkHash := range s.chunkHashes {
		copy(buf[12+i*blake2b.Size256:12+(i+1)*blake2b.Size256], chunkHash[:])
	}

	return buf
}

func UnmarshalSyncInitResponse(r io.Reader) (q SyncInitResponse, err error) {
	buf := make([]byte, 8+4)

	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}

	q.latestViewID = binary.BigEndian.Uint64(buf[0:8])
	q.chunkHashes = make([][blake2b.Size256]byte, binary.BigEndian.Uint32(buf[8:12]))

	for i := range q.chunkHashes {
		if _, err = io.ReadFull(r, q.chunkHashes[i][:]); err != nil {
			return
		}
	}

	return
}

func (s SyncChunkRequest) Marshal() []byte {
	return s.chunkHash[:]
}

func UnmarshalSyncChunkRequest(r io.Reader) (q SyncChunkRequest, err error) {
	if _, err = io.ReadFull(r, q.chunkHash[:]); err != nil {
		return
	}

	return
}

func (s SyncChunkResponse) Marshal() []byte {
	buf := make([]byte, 4+len(s.diff))

	binary.BigEndian.PutUint32(buf[0:4], uint32(len(s.diff)))
	copy(buf[4:4+len(s.diff)], s.diff)

	return buf
}

func UnmarshalSyncChunkResponse(r io.Reader) (q SyncChunkResponse, err error) {
	buf := make([]byte, 4)

	if _, err = io.ReadFull(r, buf); err != nil {
		return
	}

	q.diff = make([]byte, binary.BigEndian.Uint32(buf))

	if _, err = io.ReadFull(r, q.diff); err != nil {
		return
	}

	return
}

func (s SyncMissingTxRequest) Marshal() []byte {
	buf := make([]byte, 1+len(s.checksums)*8)
	buf[0] = byte(len(s.checksums))

	for i := range s.checksums {
		binary.BigEndian.PutUint64(buf[1+(i*8):1+(i*8+8)], s.checksums[i])
	}

	return buf
}

func UnmarshalSyncMissingTxRequest(r io.Reader) (q SyncMissingTxRequest, err error) {
	var buf [8]byte

	if _, err = io.ReadFull(r, buf[:1]); err != nil {
		return
	}

	q.checksums = make([]uint64, buf[0])

	for i := range q.checksums {
		if _, err = io.ReadFull(r, buf[:8]); err != nil {
			return
		}

		q.checksums[i] = binary.BigEndian.Uint64(buf[:8])
	}

	return
}

func (s SyncMissingTxResponse) Marshal() []byte {
	var buf []byte

	buf = append(buf, byte(len(s.transactions)))

	for _, tx := range s.transactions {
		buf = append(buf, tx.Marshal()...)
	}

	return buf
}

func UnmarshalSyncMissingTxResponse(r io.Reader) (q SyncMissingTxResponse, err error) {
	var buf [1]byte

	if _, err = io.ReadFull(r, buf[:]); err != nil {
		return
	}

	q.transactions = make([]wavelet.Transaction, buf[0])

	for i := range q.transactions {
		if q.transactions[i], err = wavelet.UnmarshalTransaction(r); err != nil {
			return
		}
	}

	return
}

type LatestViewRequest uint64

func (s LatestViewRequest) Marshal() []byte {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:8], uint64(s))

	return buf[:]
}

func UnmarshalLatestViewRequest(r io.Reader) (q LatestViewRequest, err error) {
	var buf [8]byte

	if _, err = io.ReadFull(r, buf[:4]); err != nil {
		return
	}

	q = LatestViewRequest(binary.BigEndian.Uint64(buf[:8]))

	return
}

type LatestViewResponse []uint64

func (s LatestViewResponse) Marshal() []byte {
	buf := make([]byte, 4+8*len(s))

	binary.BigEndian.PutUint32(buf[:4], uint32(len(s)))

	for i := range s {
		binary.BigEndian.PutUint64(buf[4+(i*8):4+(i*8+8)], s[i])
	}

	return buf
}

func UnmarshalLatestViewResponse(r io.Reader) (q LatestViewResponse, err error) {
	var buf [8]byte

	if _, err = io.ReadFull(r, buf[:4]); err != nil {
		return
	}

	q = make(LatestViewResponse, binary.BigEndian.Uint32(buf[:4]))

	for i := range q {
		if _, err = io.ReadFull(r, buf[:8]); err != nil {
			return
		}

		q[i] = binary.BigEndian.Uint64(buf[:8])
	}

	return
}
