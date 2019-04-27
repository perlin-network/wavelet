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

type receiverPayload struct {
	opcode byte
	wire   noise.Wire
}

type receiver struct {
	bus     chan receiverPayload
	stopped atomic.Bool

	wg     sync.WaitGroup
	cancel func()
}

func NewReceiver(protocol *Protocol, workersNum int, capacity uint32) *receiver {
	ctx, cancel := context.WithCancel(context.Background())

	r := receiver{
		bus:    make(chan receiverPayload, capacity),
		cancel: cancel,
	}

	r.wg.Add(workersNum)

	for i := 0; i < workersNum; i++ {
		go func(ctx context.Context) {
			defer r.wg.Done()

			var msg receiverPayload
			for {
				select {
				case <-ctx.Done():
					return
				case msg = <-r.bus:
				}

				switch msg.opcode {
				case protocol.opcodeGossipRequest:
					protocol.handleGossipRequest(msg.wire)
				case protocol.opcodeQueryRequest:
					protocol.handleQueryRequest(msg.wire)
				case protocol.opcodeOutOfSyncRequest:
					protocol.handleOutOfSyncCheck(msg.wire)
				case protocol.opcodeSyncInitRequest:
					protocol.handleSyncInits(msg.wire)
				case protocol.opcodeSyncChunkRequest:
					protocol.handleSyncChunks(msg.wire)
				case protocol.opcodeDownloadTxRequest:
					protocol.handleDownloadTxRequests(msg.wire)
				case protocol.opcodeForwardTxRequest:
					protocol.handleForwardTxRequest(msg.wire)
				}
			}
		}(ctx)
	}

	return &r
}

func (r *receiver) Stop() {
	if !r.stopped.CAS(false, true) {
		return
	}

	r.cancel()
	r.wg.Wait()

	close(r.bus)
}

type Protocol struct {
	opcodeGossipRequest  byte
	opcodeGossipResponse byte

	opcodeQueryRequest  byte
	opcodeQueryResponse byte

	opcodeOutOfSyncRequest  byte
	opcodeOutOfSyncResponse byte

	opcodeSyncInitRequest   byte
	opcodeSyncInitResponse  byte
	opcodeSyncChunkRequest  byte
	opcodeSyncChunkResponse byte

	opcodeDownloadTxRequest  byte
	opcodeDownloadTxResponse byte

	opcodeForwardTxRequest byte

	ledger *wavelet.Ledger

	network *skademlia.Protocol
	keys    *skademlia.Keypair

	receiverLock sync.Mutex
	receivers    []*receiver

	broadcaster *broadcaster

	ctx    context.Context
	cancel func()
	wg     sync.WaitGroup
}

func New(network *skademlia.Protocol, keys *skademlia.Keypair) *Protocol {
	return &Protocol{ledger: wavelet.NewLedger(keys), network: network, keys: keys}
}

func (p *Protocol) Stop() {
	p.ledger.Stop()
	p.cancel()
	p.wg.Wait()

	for _, r := range p.receivers {
		r.Stop()
	}

	p.broadcaster.Stop()
}

func (p *Protocol) Ledger() *wavelet.Ledger {
	return p.ledger
}

func (p *Protocol) RegisterOpcodes(node *noise.Node) {
	p.opcodeGossipRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("gossip request", p.opcodeGossipRequest)

	p.opcodeGossipResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("gossip response", p.opcodeGossipResponse)

	p.opcodeQueryRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("query request", p.opcodeQueryRequest)

	p.opcodeQueryResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("query response", p.opcodeQueryResponse)

	p.opcodeOutOfSyncRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("out of sync request", p.opcodeOutOfSyncRequest)

	p.opcodeOutOfSyncResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("out of sync response", p.opcodeOutOfSyncResponse)

	p.opcodeSyncInitRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("sync init request", p.opcodeSyncInitRequest)

	p.opcodeSyncInitResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("sync init response", p.opcodeSyncInitResponse)

	p.opcodeSyncChunkRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("sync chunk request", p.opcodeSyncChunkRequest)

	p.opcodeSyncChunkResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("sync chunk response", p.opcodeSyncChunkResponse)

	p.opcodeDownloadTxRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("download tx request", p.opcodeDownloadTxRequest)

	p.opcodeDownloadTxResponse = node.NextAvailableOpcode()
	node.RegisterOpcode("download tx response", p.opcodeDownloadTxResponse)

	p.opcodeForwardTxRequest = node.NextAvailableOpcode()
	node.RegisterOpcode("forward tx request", p.opcodeForwardTxRequest)
}

func (p *Protocol) Init(node *noise.Node) {
	p.ctx, p.cancel = context.WithCancel(context.Background())

	p.broadcaster = NewBroadcaster(12, 1000)

	go p.sendLoop(node)
	go p.ledger.Run()
}

func (p *Protocol) Protocol() noise.ProtocolBlock {
	return func(ctx noise.Context) error {
		id := ctx.Get(skademlia.KeyID).(*skademlia.ID)

		if id == nil {
			return errors.New("wavelet: user does not have a s/kademlia id registered")
		}

		publicKey := id.PublicKey()

		signal := ctx.Peer().RegisterSignal(SignalAuthenticated)
		defer signal()

		r := NewReceiver(p, 4, 1000)

		p.receiverLock.Lock()
		p.receivers = append(p.receivers, r)
		p.receiverLock.Unlock()

		go p.receiveLoop(ctx.Peer(), r.bus)

		ctx.Peer().InterceptErrors(func(err error) {
			r.Stop()

			logger := log.Network("left")
			logger.Info().
				Str("address", id.Address()).
				Hex("public_key", publicKey[:]).
				Msg("Peer has disconnected.")
		})

		logger := log.Network("joined")
		logger.Info().
			Str("address", id.Address()).
			Hex("public_key", publicKey[:]).
			Msg("Peer has joined.")

		return nil
	}
}

func (p *Protocol) sendLoop(node *noise.Node) {
	go p.broadcastGossip(node)
	go p.broadcastQueries(node)
	go p.broadcastOutOfSyncChecks(node)
	go p.broadcastSyncInitRequests(node)
	go p.broadcastSyncDiffRequests(node)
	go p.broadcastDownloadTxRequests(node)
	go p.broadcastForwardTxRequests(node)
}

func (p *Protocol) receiveLoop(peer *noise.Peer, bus chan<- receiverPayload) {
	p.wg.Add(1)
	defer p.wg.Done()

	var rp receiverPayload
	for {
		select {
		case <-p.ctx.Done():
			return
		case rp.wire = <-peer.Recv(p.opcodeGossipRequest):
			rp.opcode = p.opcodeGossipRequest
		case rp.wire = <-peer.Recv(p.opcodeQueryRequest):
			rp.opcode = p.opcodeQueryRequest
		case rp.wire = <-peer.Recv(p.opcodeOutOfSyncRequest):
			rp.opcode = p.opcodeOutOfSyncRequest
		case rp.wire = <-peer.Recv(p.opcodeSyncInitRequest):
			rp.opcode = p.opcodeSyncInitRequest
		case rp.wire = <-peer.Recv(p.opcodeSyncChunkRequest):
			rp.opcode = p.opcodeSyncChunkRequest
		case rp.wire = <-peer.Recv(p.opcodeDownloadTxRequest):
			rp.opcode = p.opcodeDownloadTxRequest
		case rp.wire = <-peer.Recv(p.opcodeForwardTxRequest):
			rp.opcode = p.opcodeForwardTxRequest
		}

		bus <- rp
	}
}

func (p *Protocol) broadcastGossip(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.GossipOut:
			peers, err := selectPeers(p.network, node, sys.SnowballQueryK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "failed to select peers while gossiping")
				continue
			}

			responses := p.broadcaster.Broadcast(
				p.ctx,
				peers,
				p.opcodeGossipRequest,
				p.opcodeGossipResponse,
				GossipRequest{tx: evt.TX}.Marshal(),
				true,
			)

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

func (p *Protocol) broadcastQueries(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.QueryOut:
			peers, err := selectPeers(p.network, node, sys.SnowballQueryK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "failed to select peers while querying")
				continue
			}

			responses := p.broadcaster.Broadcast(
				p.ctx,
				peers,
				p.opcodeQueryRequest,
				p.opcodeQueryResponse,
				QueryRequest{round: *evt.Round}.Marshal(),
				true,
			)

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

func (p *Protocol) broadcastOutOfSyncChecks(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.OutOfSyncOut:
			peers, err := selectPeers(p.network, node, sys.SnowballSyncK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while selecting peers for out of sync check")
				continue
			}

			responses := p.broadcaster.Broadcast(
				p.ctx,
				peers,
				p.opcodeOutOfSyncRequest,
				p.opcodeOutOfSyncResponse,
				nil,
				true,
			)

			votes := make([]wavelet.VoteOutOfSync, len(peers))

			for i, buf := range responses {
				if buf != nil {
					res, err := UnmarshalOutOfSyncResponse(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("Error while unmarshaling out of sync response", err)
						continue
					}

					votes[i].Round = res.round
				}

				votes[i].Voter = peers[i].Ctx().Get(skademlia.KeyID).(*skademlia.ID).PublicKey()
			}

			evt.Result <- votes

		}
	}
}

func (p *Protocol) broadcastSyncInitRequests(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.SyncInitOut:
			peers, err := selectPeers(p.network, node, sys.SnowballSyncK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while selecting peers for sync init")
				continue
			}

			responses := p.broadcaster.Broadcast(
				p.ctx,
				peers,
				p.opcodeSyncInitRequest,
				p.opcodeSyncInitResponse,
				SyncInitRequest{viewID: evt.RoundID}.Marshal(),
				true,
			)

			votes := make([]wavelet.SyncInitMetadata, len(responses))

			for i, buf := range responses {
				if buf != nil {
					res, err := UnmarshalSyncInitResponse(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("Error while unmarshaling sync init response", err)
						continue
					}

					votes[i].RoundID = res.latestViewID
					votes[i].ChunkHashes = res.chunkHashes
				}

				votes[i].PeerID = peers[i].Ctx().Get(skademlia.KeyID).(*skademlia.ID)
			}

			evt.Result <- votes
		}
	}
}

func (p *Protocol) broadcastDownloadTxRequests(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.SyncTxOut:
			peers, err := selectPeers(p.network, node, sys.SnowballSyncK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while selecting peers for syncing missing transactions")
				continue
			}

			responses := p.broadcaster.Broadcast(
				p.ctx,
				peers,
				p.opcodeDownloadTxRequest,
				p.opcodeDownloadTxResponse,
				DownloadTxRequest{ids: evt.IDs}.Marshal(),
				true,
			)

			set := make(map[common.TransactionID]wavelet.Transaction)

			for _, buf := range responses {
				if buf != nil {
					res, err := UnmarshalDownloadTxResponse(bytes.NewReader(buf))

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

func (p *Protocol) broadcastSyncDiffRequests(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.SyncDiffOut:
			collected := make([][]byte, len(evt.Sources))
			var count atomic.Uint32

			var wg sync.WaitGroup
			wg.Add(len(evt.Sources))

			for chunkID, src := range evt.Sources {
				chunkID, src := chunkID, src

				go func() {
					defer wg.Done()

					for i := 0; i < 5; i++ {
						peer := p.network.PeerByID(node, src.PeerIDs[rand.Intn(len(src.PeerIDs))])

						if peer == nil {
							continue
						}

						diff, err := func() ([]byte, error) {
							mux := peer.Mux()
							defer func() {
								_ = mux.Close()
							}()

							err := mux.SendWithTimeout(p.opcodeSyncChunkRequest, SyncChunkRequest{chunkHash: src.Hash}.Marshal(), 1*time.Second)
							if err != nil {
								return nil, err
							}

							var buf []byte

							select {
							case <-time.After(sys.QueryTimeout):
								return nil, errors.New("timed out querying for chunk")
							case wire := <-mux.Recv(p.opcodeSyncChunkResponse):
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

func (p *Protocol) broadcastForwardTxRequests(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.ForwardTXOut:
			peers, err := selectPeers(p.network, node, sys.SnowballSyncK)
			if err != nil {
				continue
			}

			p.broadcaster.Broadcast(
				p.ctx,
				peers,
				p.opcodeForwardTxRequest,
				byte(0),
				evt.TX.Marshal(),
				false,
			)
		}
	}
}

func (p *Protocol) handleQueryRequest(wire noise.Wire) {
	var res QueryResponse
	defer func() {
		if err := wire.SendWithTimeout(p.opcodeQueryResponse, res.Marshal(), 1*time.Second); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalQueryRequest(bytes.NewReader(wire.Bytes()))
	if err != nil {
		fmt.Println("Error while unmarshaling query request", err)
		return
	}

	evt := wavelet.EventIncomingQuery{Round: req.round, Response: make(chan *wavelet.Round, 1), Error: make(chan error, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending query request to ledger")
	case p.ledger.QueryIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting query result from ledger")
	case err := <-evt.Error:
		if err != nil && errors.Cause(err) != wavelet.ErrMissingParents {
			fmt.Printf("got an error processing query request from %s: %s\n", wire.Peer().Addr(), err)
		}
	case preferred := <-evt.Response:
		if preferred != nil {
			res.preferred = *preferred
		}
	}
}

func (p *Protocol) handleGossipRequest(wire noise.Wire) {
	var res GossipResponse
	defer func() {
		if err := wire.SendWithTimeout(p.opcodeGossipResponse, res.Marshal(), 1*time.Second); err != nil {
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
	case p.ledger.GossipIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting vote from ledger")
		return
	case err := <-evt.Vote:
		res.vote = err == nil

		if err != nil && errors.Cause(err) != wavelet.ErrMissingParents {
			fmt.Printf("got an error processing gossip request from %s: %s\n", wire.Peer().Addr(), err)
		}
	}
}

func (p *Protocol) handleOutOfSyncCheck(wire noise.Wire) {
	var res OutOfSyncResponse
	defer func() {
		if err := wire.SendWithTimeout(p.opcodeOutOfSyncResponse, res.Marshal(), 1*time.Second); err != nil {
			fmt.Println(err)
		}
	}()

	evt := wavelet.EventIncomingOutOfSyncCheck{Response: make(chan *wavelet.Round, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending out of sync check to ledger")
		return
	case p.ledger.OutOfSyncIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting out of sync check results from ledger")
		return
	case root := <-evt.Response:
		res.round = *root
	}
}

func (p *Protocol) handleSyncInits(wire noise.Wire) {
	var res SyncInitResponse
	defer func() {
		if err := wire.SendWithTimeout(p.opcodeSyncInitResponse, res.Marshal(), 1*time.Second); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalSyncInitRequest(bytes.NewReader(wire.Bytes()))
	if err != nil {
		fmt.Println("Error while unmarshaling sync init request", err)
		return
	}

	evt := wavelet.EventIncomingSyncInit{RoundID: req.viewID, Response: make(chan wavelet.SyncInitMetadata, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending sync init request to ledger")
		return
	case p.ledger.SyncInitIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting sync init results from ledger")
		return
	case data := <-evt.Response:
		res.chunkHashes = data.ChunkHashes
		res.latestViewID = data.RoundID
	}
}

func (p *Protocol) handleSyncChunks(wire noise.Wire) {
	var res SyncChunkResponse
	defer func() {
		if err := wire.SendWithTimeout(p.opcodeSyncChunkResponse, res.Marshal(), 1*time.Second); err != nil {
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
	case p.ledger.SyncDiffIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting sync diff results from ledger")
		return
	case chunk := <-evt.Response:
		res.diff = chunk
	}
}

func (p *Protocol) handleDownloadTxRequests(wire noise.Wire) {
	var res DownloadTxResponse
	defer func() {
		if err := wire.SendWithTimeout(p.opcodeDownloadTxResponse, res.Marshal(), 1*time.Second); err != nil {
			fmt.Println(err)
		}
	}()

	req, err := UnmarshalDownloadTxRequest(bytes.NewReader(wire.Bytes()))
	if err != nil {
		fmt.Println("Error while unmarshaling sync missing tx request", err)
		return
	}

	evt := wavelet.EventIncomingDownloadTX{IDs: req.ids, Response: make(chan []wavelet.Transaction, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending missing tx sync request to ledger")
		return
	case p.ledger.SyncTxIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting missing tx sync results from ledger")
		return
	case txs := <-evt.Response:
		res.transactions = txs
	}
}

func (p *Protocol) handleForwardTxRequest(wire noise.Wire) {
	tx, err := wavelet.UnmarshalTransaction(bytes.NewReader(wire.Bytes()))
	if err != nil {
		fmt.Println("Error while unmarshaling forward tx request", err)
		return
	}

	evt := wavelet.EventForwardTX{TX: tx}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending forward tx request to ledger")
		return
	case p.ledger.ForwardTXIn <- evt:
	}
}