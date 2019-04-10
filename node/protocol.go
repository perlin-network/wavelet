package node

import (
	"bytes"
	"context"
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"golang.org/x/crypto/blake2b"
	"math/rand"
	"sync"
	"time"
)

const (
	SignalAuthenticated = "wavelet.auth.ch"
)

type Protocol struct {
	opcodeGossipRequest  byte
	opcodeGossipResponse byte

	opcodeQueryRequest  byte
	opcodeQueryResponse byte

	opcodeSyncViewRequest  byte
	opcodeSyncViewResponse byte

	opcodeSyncInitRequest   byte
	opcodeSyncInitResponse  byte
	opcodeSyncChunkRequest  byte
	opcodeSyncChunkResponse byte

	opcodeSyncMissingTxRequest  byte
	opcodeSyncMissingTxResponse byte

	ledger *wavelet.Ledger

	network *skademlia.Protocol
	keys    *skademlia.Keypair
}

func New(network *skademlia.Protocol, keys *skademlia.Keypair) *Protocol {
	return &Protocol{ledger: wavelet.NewLedger(context.TODO(), keys, store.NewInmem()), network: network, keys: keys}
}

func (b *Protocol) Ledger() *wavelet.Ledger {
	return b.ledger
}

func (b *Protocol) RegisterOpcodes(node *noise.Node) {
	b.opcodeGossipRequest = node.NextAvailableOpcode()
	b.opcodeGossipResponse = node.NextAvailableOpcode()
	b.opcodeQueryRequest = node.NextAvailableOpcode()
	b.opcodeQueryResponse = node.NextAvailableOpcode()
	b.opcodeSyncViewRequest = node.NextAvailableOpcode()
	b.opcodeSyncViewResponse = node.NextAvailableOpcode()
	b.opcodeSyncInitRequest = node.NextAvailableOpcode()
	b.opcodeSyncInitResponse = node.NextAvailableOpcode()
	b.opcodeSyncChunkRequest = node.NextAvailableOpcode()
	b.opcodeSyncChunkResponse = node.NextAvailableOpcode()
	b.opcodeSyncMissingTxRequest = node.NextAvailableOpcode()
	b.opcodeSyncMissingTxResponse = node.NextAvailableOpcode()

	node.RegisterOpcode("gossip request", b.opcodeGossipRequest)
	node.RegisterOpcode("gossip response", b.opcodeGossipResponse)
	node.RegisterOpcode("query request", b.opcodeQueryRequest)
	node.RegisterOpcode("query response", b.opcodeQueryResponse)
	node.RegisterOpcode("sync view request", b.opcodeSyncViewRequest)
	node.RegisterOpcode("sync view response", b.opcodeSyncViewResponse)
	node.RegisterOpcode("sync init request", b.opcodeSyncInitRequest)
	node.RegisterOpcode("sync init response", b.opcodeSyncInitResponse)
	node.RegisterOpcode("sync chunk request", b.opcodeSyncChunkRequest)
	node.RegisterOpcode("sync chunk response", b.opcodeSyncChunkResponse)
	node.RegisterOpcode("sync missing tx request", b.opcodeSyncMissingTxRequest)
	node.RegisterOpcode("sync missing tx response", b.opcodeSyncMissingTxResponse)
}

func (b *Protocol) Init(node *noise.Node) {
	go b.sendLoop(node)
	go wavelet.Run(b.ledger)
}

func (b *Protocol) Protocol() noise.ProtocolBlock {
	return func(ctx noise.Context) error {
		id := ctx.Get(skademlia.KeyID).(*skademlia.ID)

		if id == nil {
			return errors.New("wavelet: user does not have a s/kademlia id registered")
		}

		signal := ctx.Peer().RegisterSignal(SignalAuthenticated)
		defer signal()

		ctx.Peer().InterceptErrors(func(err error) {
			logger := log.Network("left")
			logger.Info().
				Str("address", id.Address()).
				Msg("Peer has disconnected.")
		})

		go b.receiveLoop(b.ledger, ctx)

		logger := log.Network("joined")
		logger.Info().
			Str("address", id.Address()).
			Msg("Peer has joined.")

		return nil
	}
}

func (b *Protocol) sendLoop(node *noise.Node) {
	ctx := context.TODO()

	go b.broadcastGossip(ctx, node)
	go b.broadcastQueries(ctx, node)
	go b.broadcastOutOfSyncChecks(ctx, node)
	go b.broadcastSyncInitRequests(ctx, node)
	go b.broadcastSyncDiffRequests(ctx, node)
	go b.broadcastSyncMissingTXs(ctx, node)
}

func (b *Protocol) receiveLoop(ledger *wavelet.Ledger, ctx noise.Context) {
	peer := ctx.Peer()

	for {
		select {
		case wire := <-peer.Recv(b.opcodeGossipRequest):
			go b.handleGossipRequest(wire)
		case wire := <-peer.Recv(b.opcodeQueryRequest):
			go b.handleQueryRequest(wire)
		case wire := <-peer.Recv(b.opcodeSyncViewRequest):
			go b.handleOutOfSyncCheck(wire)
		case wire := <-peer.Recv(b.opcodeSyncInitRequest):
			go b.handleSyncInits(wire)
		case wire := <-peer.Recv(b.opcodeSyncChunkRequest):
			go b.handleSyncChunks(wire)
		case wire := <-peer.Recv(b.opcodeSyncMissingTxRequest):
			go b.handleSyncMissingTXs(wire)
		}
	}
}

func (b *Protocol) broadcastGossip(ctx context.Context, node *noise.Node) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-b.ledger.GossipOut:
			peers, err := selectPeers(b.network, node, sys.SnowballQueryK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "failed to select peers while gossiping")
				return
			}

			responses, err := broadcast(peers, b.opcodeGossipRequest, b.opcodeGossipResponse, GossipRequest{tx: evt.TX}.Marshal())
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while gossiping")
				return
			}

			votes := make([]wavelet.VoteGossip, len(responses))

			for i, buf := range responses {
				res, err := UnmarshalGossipResponse(bytes.NewReader(buf))

				if err != nil {
					continue
				}

				id := peers[i].Ctx().Get(skademlia.KeyID).(*skademlia.ID)

				votes[i] = wavelet.VoteGossip{
					Voter: id.PublicKey(),
					Ok:    res.vote,
				}
			}

			evt.Result <- votes
		}
	}
}

func (b *Protocol) broadcastQueries(ctx context.Context, node *noise.Node) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-b.ledger.QueryOut:
			peers, err := selectPeers(b.network, node, sys.SnowballQueryK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "failed to select peers while querying")
				return
			}

			responses, err := broadcast(peers, b.opcodeQueryRequest, b.opcodeQueryResponse, QueryRequest{tx: evt.TX}.Marshal())
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while querying")
				return
			}

			votes := make([]wavelet.VoteQuery, len(responses))

			for i, buf := range responses {
				res, err := UnmarshalQueryResponse(bytes.NewReader(buf))

				if err != nil {
					continue
				}

				id := peers[i].Ctx().Get(skademlia.KeyID).(*skademlia.ID)

				votes[i] = wavelet.VoteQuery{Voter: id.PublicKey(), Preferred: res.preferred}
			}

			evt.Result <- votes
		}
	}
}

func (b *Protocol) broadcastOutOfSyncChecks(ctx context.Context, node *noise.Node) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-b.ledger.OutOfSyncOut:
			peers, err := selectPeers(b.network, node, sys.SnowballSyncK)
			if err != nil {
				return
			}

			responses, err := broadcast(peers, b.opcodeSyncViewRequest, b.opcodeSyncViewResponse, SyncViewRequest{root: evt.Root}.Marshal())
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while checking if out of sync")
				return
			}

			votes := make([]wavelet.VoteOutOfSync, len(responses))

			for i, buf := range responses {
				res, err := UnmarshalSyncViewResponse(bytes.NewReader(buf))

				if err != nil {
					continue
				}

				id := peers[i].Ctx().Get(skademlia.KeyID).(*skademlia.ID)

				votes[i] = wavelet.VoteOutOfSync{Voter: id.PublicKey(), Root: res.root}
			}

			evt.Result <- votes

		}
	}
}

func (b *Protocol) broadcastSyncInitRequests(ctx context.Context, node *noise.Node) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-b.ledger.SyncInitOut:
			peers, err := selectPeers(b.network, node, sys.SnowballSyncK)
			if err != nil {
				return
			}

			responses, err := broadcast(peers, b.opcodeSyncInitRequest, b.opcodeSyncInitResponse, SyncInitRequest{viewID: evt.ViewID}.Marshal())
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while sending request to sync")
				return
			}

			votes := make([]wavelet.SyncInitMetadata, len(responses))

			for i, buf := range responses {
				res, err := UnmarshalSyncInitResponse(bytes.NewReader(buf))

				if err != nil {
					continue
				}

				id := peers[i].Ctx().Get(skademlia.KeyID).(*skademlia.ID)

				votes[i] = wavelet.SyncInitMetadata{
					PeerID:      id,
					ViewID:      res.latestViewID,
					ChunkHashes: res.chunkHashes,
				}
			}

			evt.Result <- votes
		}
	}
}

func (b *Protocol) broadcastSyncMissingTXs(ctx context.Context, node *noise.Node) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-b.ledger.SyncTxOut:
			peers, err := selectPeers(b.network, node, sys.SnowballSyncK)
			if err != nil {
				return
			}

			responses, err := broadcast(peers, b.opcodeSyncMissingTxRequest, b.opcodeSyncMissingTxResponse, SyncMissingTxRequest{ids: evt.IDs}.Marshal())
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while sending request for missing transactions")
				return
			}

			set := make(map[common.TransactionID]wavelet.Transaction)

			for _, buf := range responses {
				res, err := UnmarshalSyncMissingTxResponse(bytes.NewReader(buf))

				if err != nil {
					continue
				}

				for _, tx := range res.transactions {
					set[tx.ID] = tx
				}
			}

			var txs []wavelet.Transaction

			for _, tx := range set {
				txs = append(txs, tx)
			}

			evt.Result <- txs
		}
	}
}

func (b *Protocol) broadcastSyncDiffRequests(ctx context.Context, node *noise.Node) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-b.ledger.SyncDiffOut:
			collected := make([][]byte, len(evt.Sources))
			var count atomic.Uint32

			var wg sync.WaitGroup
			wg.Add(len(evt.Sources))

			for chunkID, src := range evt.Sources {
				src := src

				go func() {
					defer wg.Done()

					for i := 0; i < 5; i++ {
						func() {
							peer := b.network.PeerByID(node, src.PeerIDs[rand.Intn(len(src.PeerIDs))])

							if peer == nil {
								return
							}

							mux := peer.Mux()
							defer func() {
								_ = mux.Close()
							}()

							err := mux.Send(b.opcodeSyncChunkRequest, SyncChunkRequest{chunkHash: src.Hash}.Marshal())
							if err != nil {
								return
							}

							var buf []byte

							select {
							case <-time.After(sys.QueryTimeout):
								return
							case wire := <-mux.Recv(b.opcodeSyncChunkResponse):
								buf = wire.Bytes()
							}

							res, err := UnmarshalSyncChunkResponse(bytes.NewReader(buf))

							if err != nil {
								return
							}

							if res.diff == nil || blake2b.Sum256(res.diff) != src.Hash {
								return
							}

							collected[chunkID] = res.diff

							count.Add(1)
						}()
					}
				}()
			}

			wg.Wait()

			if int(count.Load()) != len(evt.Sources) {
				evt.Error <- errors.New("failed to fetch some chunks from our peers")
				return
			}

			evt.Result <- collected
		}
	}
}

func (b *Protocol) handleQueryRequest(wire noise.Wire) {
	var res QueryResponse
	defer func() {
		if err := wire.Send(b.opcodeQueryResponse, res.Marshal()); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalQueryRequest(bytes.NewReader(wire.Bytes()))

	if err != nil {
		return
	}

	evt := wavelet.EventIncomingQuery{TX: req.tx, Response: make(chan *wavelet.Transaction, 1), Error: make(chan error, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending query request to ledger")
	case b.ledger.QueryIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting query result from ledger")
	case err := <-evt.Error:
		if err != nil {
			fmt.Printf("got an error processing query request from %s: %s\n", wire.Peer().Addr(), err)
		}
	case res.preferred = <-evt.Response:
	}
}

func (b *Protocol) handleGossipRequest(wire noise.Wire) {
	var res GossipResponse
	defer func() {
		if err := wire.Send(b.opcodeGossipResponse, res.Marshal()); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalGossipRequest(bytes.NewReader(wire.Bytes()))

	if err != nil {
		return
	}

	evt := wavelet.EventIncomingGossip{TX: req.tx, Vote: make(chan error, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending gossip request to ledger")
		return
	case b.ledger.GossipIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting vote from ledger")
		return
	case err := <-evt.Vote:
		res.vote = err == nil

		if err != nil {
			fmt.Printf("got an error processing gossip request from %s: %s\n", wire.Peer().Addr(), err)
		}
	}
}

func (b *Protocol) handleOutOfSyncCheck(wire noise.Wire) {
	var res SyncViewResponse
	defer func() {
		if err := wire.Send(b.opcodeSyncViewResponse, res.Marshal()); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalSyncViewRequest(bytes.NewReader(wire.Bytes()))

	if err != nil {
		return
	}

	evt := wavelet.EventIncomingOutOfSyncCheck{Root: req.root, Response: make(chan *wavelet.Transaction, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending out of sync check to ledger")
		return
	case b.ledger.OutOfSyncIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting sync check results from ledger")
		return
	case root := <-evt.Response:
		res.root = *root
	}
}

func (b *Protocol) handleSyncInits(wire noise.Wire) {
	var res SyncInitResponse
	defer func() {
		if err := wire.Send(b.opcodeSyncInitResponse, res.Marshal()); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalSyncInitRequest(bytes.NewReader(wire.Bytes()))

	if err != nil {
		return
	}

	evt := wavelet.EventIncomingSyncInit{ViewID: req.viewID, Response: make(chan wavelet.SyncInitMetadata, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending sync init request to ledger")
		return
	case b.ledger.SyncInitIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting sync init results from ledger")
		return
	case data := <-evt.Response:
		res.chunkHashes = data.ChunkHashes
		res.latestViewID = data.ViewID
	}
}

func (b *Protocol) handleSyncChunks(wire noise.Wire) {
	var res SyncChunkResponse
	defer func() {
		if err := wire.Send(b.opcodeSyncChunkResponse, res.Marshal()); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalSyncChunkRequest(bytes.NewReader(wire.Bytes()))

	if err != nil {
		return
	}

	evt := wavelet.EventIncomingSyncDiff{ChunkHash: req.chunkHash, Response: make(chan []byte, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending sync diff request to ledger")
		return
	case b.ledger.SyncDiffIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting sync diff results from ledger")
		return
	case chunk := <-evt.Response:
		res.diff = chunk
	}
}

func (b *Protocol) handleSyncMissingTXs(wire noise.Wire) {
	var res SyncMissingTxResponse
	defer func() {
		if err := wire.Send(b.opcodeSyncMissingTxResponse, res.Marshal()); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalSyncMissingTxRequest(bytes.NewReader(wire.Bytes()))

	if err != nil {
		return
	}

	evt := wavelet.EventIncomingSyncTX{IDs: req.ids, Response: make(chan []wavelet.Transaction, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending missing tx sync request to ledger")
		return
	case b.ledger.SyncTxIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting missing tx sync results from ledger")
		return
	case txs := <-evt.Response:
		res.transactions = txs
	}
}
