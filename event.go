package wavelet

import (
	"context"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/common"
	"golang.org/x/crypto/blake2b"
	"time"
)

const (
	SyncChunkSize = 1048576
)

type Event interface {
	Result(ctx context.Context, timeout time.Duration) (interface{}, error)
}

type EventBroadcast struct {
	tag       byte
	payload   []byte
	creator   common.AccountID
	signature common.Signature

	result chan Transaction
	error  chan error
}

func NewEventBroadcast(tag byte, payload []byte, creator common.AccountID, signature common.Signature) EventBroadcast {
	return EventBroadcast{
		tag:       tag,
		payload:   payload,
		creator:   creator,
		signature: signature,
		result:    make(chan Transaction, 1),
		error:     make(chan error, 1),
	}
}

func (eb EventBroadcast) Result(ctx context.Context, timeout time.Duration) (interface{}, error) {
	var res Transaction
	select {
	case <-ctx.Done():
		return res, ErrStopped
	case <-time.After(timeout):
		return res, ErrTimeout
	case res = <- eb.result:
		return res, nil
	case err := <- eb.error:
		return res, err
	}
}

type EventIncomingGossip struct {
	TX Transaction

	Vote chan error
}

type VoteGossip struct {
	Voter common.AccountID
	Ok    bool
}

type EventGossip struct {
	TX Transaction

	Result chan []VoteGossip
	Error  chan error
}

type EventIncomingQuery struct {
	Round Round

	Response chan *Round
	Error    chan error
}

type VoteQuery struct {
	Voter     common.AccountID
	Preferred Round
}

type EventQuery struct {
	Round *Round

	Result chan []VoteQuery
	Error  chan error
}

type VoteOutOfSync struct {
	Voter common.AccountID
	Round Round
}

type EventOutOfSyncCheck struct {
	Result chan []VoteOutOfSync
	Error  chan error
}

type EventIncomingOutOfSyncCheck struct {
	Response chan *Round
}

type SyncInitMetadata struct {
	PeerID      *skademlia.ID
	RoundID     uint64
	ChunkHashes [][blake2b.Size256]byte
}

type EventSyncInit struct {
	RoundID uint64

	Result chan []SyncInitMetadata
	Error  chan error
}

type EventIncomingSyncInit struct {
	RoundID uint64

	Response chan SyncInitMetadata
}

type ChunkSource struct {
	Hash    [blake2b.Size256]byte
	PeerIDs []*skademlia.ID
}

type EventSyncDiff struct {
	Sources []ChunkSource

	Result chan [][]byte
	Error  chan error
}

type EventDownloadTX struct {
	IDs []common.TransactionID

	Result chan []Transaction
	Error  chan error
}

type EventIncomingDownloadTX struct {
	IDs []common.TransactionID

	Response chan []Transaction
}

type EventIncomingSyncDiff struct {
	ChunkHash [blake2b.Size256]byte

	Response chan []byte
}

type EventLatestView struct {
	RoundID uint64
	Result  chan []uint64
	Error   chan error
}

type EventIncomingLatestView struct {
	RoundID  uint64
	Response chan []uint64
}
