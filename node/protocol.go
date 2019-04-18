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

	opcodeLatestViewRequest  byte
	opcodeLatestViewResponse byte

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
	node.RegisterOpcode("gossip request", b.opcodeGossipRequest)

	b.opcodeGossipResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("gossip response", b.opcodeGossipResponse)

	b.opcodeQueryRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("query request", b.opcodeQueryRequest)

	b.opcodeQueryResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("query response", b.opcodeQueryResponse)

	b.opcodeSyncViewRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("sync view request", b.opcodeSyncViewRequest)

	b.opcodeSyncViewResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("sync view response", b.opcodeSyncViewResponse)

	b.opcodeSyncInitRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("sync init request", b.opcodeSyncInitRequest)

	b.opcodeSyncInitResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("sync init response", b.opcodeSyncInitResponse)

	b.opcodeSyncChunkRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("sync chunk request", b.opcodeSyncChunkRequest)

	b.opcodeSyncChunkResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("sync chunk response", b.opcodeSyncChunkResponse)

	b.opcodeSyncMissingTxRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("sync missing tx request", b.opcodeSyncMissingTxRequest)

	b.opcodeSyncMissingTxResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("sync missing tx response", b.opcodeSyncMissingTxResponse)

	b.opcodeLatestViewRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("latest view request", b.opcodeLatestViewRequest)

	b.opcodeLatestViewResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("latest view response", b.opcodeLatestViewResponse)
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

		publicKey := id.PublicKey()

		signal := ctx.Peer().RegisterSignal(SignalAuthenticated)
		defer signal()

		ctx.Peer().InterceptErrors(func(err error) {
			logger := log.Network("left")
			logger.Info().
				Str("address", id.Address()).
				Hex("public_key", publicKey[:]).
				Msg("Peer has disconnected.")
		})

		go b.receiveLoop(b.ledger, ctx)

		logger := log.Network("joined")
		logger.Info().
			Str("address", id.Address()).
			Hex("public_key", publicKey[:]).
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
	go b.broadcastLatestViewRequests(ctx, node)
}

func (b *Protocol) receiveLoop(ledger *wavelet.Ledger, ctx noise.Context) {
	peer := ctx.Peer()

	for {
		select {
		case <-ctx.Done():
			return
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
		case wire := <-peer.Recv(b.opcodeLatestViewRequest):
			go b.handleLatestViewRequest(wire)
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
				continue
			}

			responses := broadcast(peers, b.opcodeGossipRequest, b.opcodeGossipResponse, GossipRequest{tx: evt.TX}.Marshal())

			votes := make([]wavelet.VoteGossip, len(responses))

			for i, buf := range responses {
				if buf != nil {
					res, err := UnmarshalGossipResponse(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("Error while unmarshaling gossip response", err)
						continue
					}

					votes[i].Ok = res.vote
				}

				votes[i].Voter = peers[i].Ctx().Get(skademlia.KeyID).(*skademlia.ID).PublicKey()
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
				continue
			}

			responses := broadcast(peers, b.opcodeQueryRequest, b.opcodeQueryResponse, QueryRequest{tx: evt.TX}.Marshal())

			votes := make([]wavelet.VoteQuery, len(responses))

			for i, buf := range responses {
				if buf != nil {
					res, err := UnmarshalQueryResponse(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("Error while unmarshaling query response", err)
						continue
					}

					votes[i].Preferred = res.preferred
				}

				votes[i].Voter = peers[i].Ctx().Get(skademlia.KeyID).(*skademlia.ID).PublicKey()
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
				evt.Error <- errors.Wrap(err, "got an error while selecting peers for sync check")
				continue
			}

			responses := broadcast(peers, b.opcodeSyncViewRequest, b.opcodeSyncViewResponse, SyncViewRequest{root: evt.Root}.Marshal())

			votes := make([]wavelet.VoteOutOfSync, len(peers))

			for i, buf := range responses {
				if buf != nil {
					res, err := UnmarshalSyncViewResponse(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("Error while unmarshaling sync view response", err)
						continue
					}

					votes[i].Root = res.root
				}

				votes[i].Voter = peers[i].Ctx().Get(skademlia.KeyID).(*skademlia.ID).PublicKey()
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
				evt.Error <- errors.Wrap(err, "got an error while selecting peers for sync init")
				continue
			}

			responses := broadcast(peers, b.opcodeSyncInitRequest, b.opcodeSyncInitResponse, SyncInitRequest{viewID: evt.ViewID}.Marshal())

			votes := make([]wavelet.SyncInitMetadata, len(responses))

			for i, buf := range responses {
				if buf != nil {
					res, err := UnmarshalSyncInitResponse(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("Error while unmarshaling sync init response", err)
						continue
					}

					votes[i].ViewID = res.latestViewID
					votes[i].ChunkHashes = res.chunkHashes
				}

				votes[i].PeerID = peers[i].Ctx().Get(skademlia.KeyID).(*skademlia.ID)
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
				evt.Error <- errors.Wrap(err, "got an error while selecting peers for syncing missing transactions")
				continue
			}

			responses := broadcast(peers, b.opcodeSyncMissingTxRequest, b.opcodeSyncMissingTxResponse, SyncMissingTxRequest{checksums: evt.Checksums}.Marshal())

			set := make(map[common.TransactionID]wavelet.Transaction)

			for _, buf := range responses {
				if buf != nil {
					res, err := UnmarshalSyncMissingTxResponse(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("Error while unmarshaling sync missing tx response", err)
						continue
					}

					for _, tx := range res.transactions {
						set[tx.ID] = tx
					}
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

func (b *Protocol) broadcastLatestViewRequests(ctx context.Context, node *noise.Node) {
	for {
		select {
		case <-ctx.Done():
			return
		case evt := <-b.ledger.LatestViewOut:
			peers, err := selectPeers(b.network, node, sys.SnowballSyncK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while selecting peers for finding missing transactions")
				continue
			}

			responses := broadcast(peers, b.opcodeLatestViewRequest, b.opcodeLatestViewResponse, LatestViewRequest(evt.ViewID).Marshal())

			set := make(map[uint64]struct{})

			for _, buf := range responses {
				if buf != nil {
					checksums, err := UnmarshalLatestViewResponse(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("Error while unmarshaling latest view response", err)
						continue
					}

					for _, checksum := range checksums {
						set[checksum] = struct{}{}
					}
				}
			}

			var checksums []uint64

			for checksum := range set {
				checksums = append(checksums, checksum)
			}

			evt.Result <- checksums
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
				chunkID, src := chunkID, src

				go func() {
					defer wg.Done()

					for i := 0; i < 5; i++ {
						peer := b.network.PeerByID(node, src.PeerIDs[rand.Intn(len(src.PeerIDs))])

						if peer == nil {
							continue
						}

						diff, err := func() ([]byte, error) {
							mux := peer.Mux()
							defer func() {
								_ = mux.Close()
							}()

							err := mux.SendWithTimeout(b.opcodeSyncChunkRequest, SyncChunkRequest{chunkHash: src.Hash}.Marshal(), 1*time.Second)
							if err != nil {
								return nil, err
							}

							var buf []byte

							select {
							case <-time.After(sys.QueryTimeout):
								return nil, errors.New("timed out querying for chunk")
							case wire := <-mux.Recv(b.opcodeSyncChunkResponse):
								buf = wire.Bytes()
							}

							res, err := UnmarshalSyncChunkResponse(bytes.NewReader(buf))

							if err != nil {
								return nil, err
							}

							if res.diff == nil || blake2b.Sum256(res.diff) != src.Hash {
								return nil, errors.New("sync chunk response was empty")
							}

							return res.diff, nil
						}()

						if err != nil {
							continue
						}

						collected[chunkID] = diff
						count.Add(1)

						break
					}
				}()
			}

			wg.Wait()

			if int(count.Load()) != len(evt.Sources) {
				evt.Error <- errors.New("failed to fetch some chunks from our peers")
				continue
			}

			evt.Result <- collected
		}
	}
}

func (b *Protocol) handleQueryRequest(wire noise.Wire) {
	var res QueryResponse
	defer func() {
		if err := wire.SendWithTimeout(b.opcodeQueryResponse, res.Marshal(), 1*time.Second); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalQueryRequest(bytes.NewReader(wire.Bytes()))
	if err != nil {
		fmt.Println("Error while unmarshaling query request", err)
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
	case preferred := <-evt.Response:
		if preferred != nil {
			res.preferred = *preferred
		}
	}
}

func (b *Protocol) handleGossipRequest(wire noise.Wire) {
	var res GossipResponse
	defer func() {
		if err := wire.SendWithTimeout(b.opcodeGossipResponse, res.Marshal(), 1*time.Second); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalGossipRequest(bytes.NewReader(wire.Bytes()))
	if err != nil {
		fmt.Println("Error while unmarshaling gossip request", err)
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
		if err := wire.SendWithTimeout(b.opcodeSyncViewResponse, res.Marshal(), 1*time.Second); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalSyncViewRequest(bytes.NewReader(wire.Bytes()))
	if err != nil {
		fmt.Println("Error while unmarshaling sync view request", err)
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
		if err := wire.SendWithTimeout(b.opcodeSyncInitResponse, res.Marshal(), 1*time.Second); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalSyncInitRequest(bytes.NewReader(wire.Bytes()))
	if err != nil {
		fmt.Println("Error while unmarshaling sync init request", err)
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
		if err := wire.SendWithTimeout(b.opcodeSyncChunkResponse, res.Marshal(), 1*time.Second); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalSyncChunkRequest(bytes.NewReader(wire.Bytes()))
	if err != nil {
		fmt.Println("Error while unmarshaling sync chunk request", err)
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

func (b *Protocol) handleLatestViewRequest(wire noise.Wire) {
	var res LatestViewResponse
	defer func() {
		if err := wire.SendWithTimeout(b.opcodeLatestViewResponse, res.Marshal(), 1*time.Second); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalLatestViewRequest(bytes.NewReader(wire.Bytes()))
	if err != nil {
		fmt.Println("Error while unmarshaling latest view request", err)
		return
	}

	evt := wavelet.EventIncomingLatestView{ViewID: uint64(req), Response: make(chan []uint64, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending latest view request to ledger")
		return
	case b.ledger.LatestViewIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting latest view response from ledger")
		return
	case checksums := <-evt.Response:
		res = checksums
	}
}

func (b *Protocol) handleSyncMissingTXs(wire noise.Wire) {
	var res SyncMissingTxResponse
	defer func() {
		if err := wire.SendWithTimeout(b.opcodeSyncMissingTxResponse, res.Marshal(), 1*time.Second); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalSyncMissingTxRequest(bytes.NewReader(wire.Bytes()))
	if err != nil {
		fmt.Println("Error while unmarshaling sync missing tx request", err)
		return
	}

	evt := wavelet.EventIncomingSyncTX{Checksums: req.checksums, Response: make(chan []wavelet.Transaction, 1)}

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
