package wavelet

import (
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/common"
	"golang.org/x/crypto/blake2b"
)

const (
	SyncChunkSize = 1048576
)

type EventBroadcast struct {
	Tag       byte
	Payload   []byte
	Creator   common.AccountID
	Signature common.Signature

	Result chan Transaction
	Error  chan error
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
	Root  Transaction
}

type EventOutOfSyncCheck struct {
	Root Transaction

	Result chan []VoteOutOfSync
	Error  chan error
}

type EventIncomingOutOfSyncCheck struct {
	Root Transaction

	Response chan *Transaction
}

type SyncInitMetadata struct {
	PeerID      *skademlia.ID
	ViewID      uint64
	ChunkHashes [][blake2b.Size256]byte
}

type EventSyncInit struct {
	ViewID uint64

	Result chan []SyncInitMetadata
	Error  chan error
}

type EventIncomingSyncInit struct {
	ViewID uint64

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

type EventSyncTX struct {
	Checksums []uint64

	Result chan []Transaction
	Error  chan error
}

type EventIncomingSyncTX struct {
	Checksums []uint64

	Response chan []Transaction
}

type EventIncomingSyncDiff struct {
	ChunkHash [blake2b.Size256]byte

	Response chan []byte
}

type EventLatestView struct {
	ViewID uint64
	Result chan []uint64
	Error  chan error
}

type EventIncomingLatestView struct {
	ViewID   uint64
	Response chan []uint64
}
