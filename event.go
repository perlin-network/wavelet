package wavelet

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/common"
	"golang.org/x/crypto/blake2b"
)

const (
	SyncChunkSize = 1048576
)

type EventGossip struct {
	TX Transaction
}

type EventBroadcast struct {
	Tag       byte
	Payload   []byte
	Creator   common.AccountID
	Signature common.Signature

	Result chan Transaction
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
