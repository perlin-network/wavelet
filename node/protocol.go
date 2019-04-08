package node

import (
	"context"
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"golang.org/x/crypto/blake2b"
	"math/rand"
	"strconv"
	"sync"
	"time"
)

var _ protocol.Block = (*block)(nil)

const (
	keyLedger      = "wavelet.ledger"
	keyAuthChannel = "wavelet.auth.ch"
)

type receiver struct {
	bus chan noise.Message

	wg sync.WaitGroup
	cancel func()
}

func NewReceiver(
	ledger *wavelet.Ledger,
	peer *noise.Peer,
	workersNum int,
	bufferSize uint32,
) *receiver {
	ctx, cancel := context.WithCancel(context.Background())

	r := receiver{
		bus: make(chan noise.Message, bufferSize),
		wg: sync.WaitGroup{},
		cancel: cancel,
	}

	r.wg.Add(workersNum)

	for i := 0; i < workersNum; i++ {
		go func(ctx context.Context) {
			defer r.wg.Done()

			var msg noise.Message
			for {
				select {
					case <-ctx.Done():
						return
					case msg =<- r.bus:
				}

				switch msg.(type) {
				case GossipRequest:
					handleGossipRequest(ledger, peer, msg.(GossipRequest))
				case QueryRequest:
					handleQueryRequest(ledger, peer, msg.(QueryRequest))
				case SyncViewRequest:
					handleOutOfSyncCheck(ledger, peer, msg.(SyncViewRequest))
				case SyncInitRequest:
					handleSyncInits(ledger, peer, msg.(SyncInitRequest))
				case SyncChunkRequest:
					handleSyncChunks(ledger, peer, msg.(SyncChunkRequest))
				case SyncMissingTxRequest:
					handleSyncMissingTXs(ledger, peer, msg.(SyncMissingTxRequest))
				}
			}
		}(ctx)
	}

	return &r
}

func (r *receiver) Stop() {
	r.cancel()
	r.wg.Wait()

	close(r.bus)
}

type block struct {
	opcodeGossipRequest  noise.Opcode
	opcodeGossipResponse noise.Opcode

	opcodeQueryRequest  noise.Opcode
	opcodeQueryResponse noise.Opcode

	opcodeSyncViewRequest  noise.Opcode
	opcodeSyncViewResponse noise.Opcode

	opcodeSyncInitRequest   noise.Opcode
	opcodeSyncInitResponse  noise.Opcode
	opcodeSyncChunkRequest  noise.Opcode
	opcodeSyncChunkResponse noise.Opcode

	opcodeSyncMissingTxRequest  noise.Opcode
	opcodeSyncMissingTxResponse noise.Opcode
}

func New() *block {
	return &block{}
}

func (b *block) OnRegister(p *protocol.Protocol, node *noise.Node) {
	b.opcodeGossipRequest = noise.RegisterMessage(noise.NextAvailableOpcode(), (*GossipRequest)(nil))
	b.opcodeGossipResponse = noise.RegisterMessage(noise.NextAvailableOpcode(), (*GossipResponse)(nil))
	b.opcodeQueryRequest = noise.RegisterMessage(noise.NextAvailableOpcode(), (*QueryRequest)(nil))
	b.opcodeQueryResponse = noise.RegisterMessage(noise.NextAvailableOpcode(), (*QueryResponse)(nil))
	b.opcodeSyncViewRequest = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncViewRequest)(nil))
	b.opcodeSyncViewResponse = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncViewResponse)(nil))
	b.opcodeSyncInitRequest = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncInitRequest)(nil))
	b.opcodeSyncInitResponse = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncInitResponse)(nil))
	b.opcodeSyncChunkRequest = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncChunkRequest)(nil))
	b.opcodeSyncChunkResponse = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncChunkResponse)(nil))
	b.opcodeSyncMissingTxRequest = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncMissingTxRequest)(nil))
	b.opcodeSyncMissingTxResponse = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncMissingTxResponse)(nil))

	kv := store.NewInmem()

	ledger := wavelet.NewLedger(node.Keys, kv)
	node.Set(keyLedger, ledger)

	go wavelet.Run(ledger)
}

func (b *block) OnBegin(p *protocol.Protocol, peer *noise.Peer) error {
	r := NewReceiver(Ledger(peer.Node()), peer, 4, 1000)

	peer.OnDisconnect(func(node *noise.Node, peer *noise.Peer) error {
		r.Stop()
		return nil
	})

	go b.sendLoop(Ledger(peer.Node()), peer)
	go b.receiveLoop(r.bus, peer)

	close(peer.LoadOrStore(keyAuthChannel, make(chan struct{})).(chan struct{}))

	return nil
}

func (b *block) sendLoop(ledger *wavelet.Ledger, peer *noise.Peer) {
	go b.broadcastGossip(ledger, peer.Node(), peer)
	go b.broadcastQueries(ledger, peer.Node(), peer)
	go b.broadcastOutOfSyncChecks(ledger, peer.Node(), peer)
	go b.broadcastSyncInitRequests(ledger, peer.Node(), peer)
	go b.broadcastSyncDiffRequests(ledger, peer.Node(), peer)
	go b.broadcastSyncMissingTXs(ledger, peer.Node(), peer)
}

func (b *block) receiveLoop(bus chan<- noise.Message, peer *noise.Peer) {
	var msg noise.Message
	for {
		select {
		case msg = <-peer.Receive(b.opcodeGossipRequest):
		case msg = <-peer.Receive(b.opcodeQueryRequest):
		case msg = <-peer.Receive(b.opcodeSyncViewRequest):
		case msg = <-peer.Receive(b.opcodeSyncInitRequest):
		case msg = <-peer.Receive(b.opcodeSyncChunkRequest):
		case msg = <-peer.Receive(b.opcodeSyncMissingTxRequest):
		}

		bus<- msg
	}
}

func (b *block) broadcastGossip(ledger *wavelet.Ledger, node *noise.Node, peer *noise.Peer) {
	for evt := range ledger.GossipOut {
		func() {
			peers, err := selectPeers(node, sys.SnowballQueryK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "failed to select peers while gossiping")
				return
			}

			responses, err := broadcast(node, peers, GossipRequest{TX: &evt.TX}, b.opcodeGossipResponse)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while gossiping")
				return
			}

			var votes []wavelet.VoteGossip

			for i, res := range responses {
				if res != nil {
					res := res.(GossipResponse)

					var voter common.AccountID
					copy(voter[:], peers[i].PublicKey())

					votes = append(votes, wavelet.VoteGossip{
						Voter: voter,
						Ok:    res.vote,
					})
				}
			}

			evt.Result <- votes
		}()
	}
}

func (b *block) broadcastQueries(ledger *wavelet.Ledger, node *noise.Node, peer *noise.Peer) {
	for evt := range ledger.QueryOut {
		func() {
			peers, err := selectPeers(node, sys.SnowballQueryK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "failed to select peers while querying")
				return
			}

			responses, err := broadcast(node, peers, QueryRequest{tx: &evt.TX}, b.opcodeQueryResponse)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while querying")
				return
			}

			var votes []wavelet.VoteQuery

			for i, res := range responses {
				if res != nil {
					res := res.(QueryResponse)

					var voter common.AccountID
					copy(voter[:], peers[i].PublicKey())

					vote := wavelet.VoteQuery{Voter: voter}

					if res.preferred != nil {
						vote.Preferred = *res.preferred
					}

					votes = append(votes, vote)
				}
			}

			evt.Result <- votes
		}()
	}
}

func (b *block) broadcastOutOfSyncChecks(ledger *wavelet.Ledger, node *noise.Node, peer *noise.Peer) {
	for evt := range ledger.OutOfSyncOut {
		func() {
			peers, err := selectPeers(node, sys.SnowballSyncK)
			if err != nil {
				// Do not send an error if we do not have any peers to broadcast out-of-sync checks to.
				return
			}

			responses, err := broadcast(node, peers, SyncViewRequest{root: &evt.Root}, b.opcodeSyncViewResponse)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while checking if out of sync")
				return
			}

			var votes []wavelet.VoteOutOfSync

			for i, res := range responses {
				if res != nil {
					res := res.(SyncViewResponse)

					var voter common.AccountID
					copy(voter[:], peers[i].PublicKey())

					vote := wavelet.VoteOutOfSync{Voter: voter}

					if res.root != nil {
						vote.Root = *res.root
					}

					votes = append(votes, vote)
				}
			}

			evt.Result <- votes
		}()
	}
}

func (b *block) broadcastSyncInitRequests(ledger *wavelet.Ledger, node *noise.Node, peer *noise.Peer) {
	for evt := range ledger.SyncInitOut {
		func() {
			peers, err := selectPeers(node, sys.SnowballSyncK)
			if err != nil {
				return
			}

			responses, err := broadcast(node, peers, SyncInitRequest{viewID: evt.ViewID}, b.opcodeSyncInitResponse)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while sending request to sync")
				return
			}

			var votes []wavelet.SyncInitMetadata

			for i, res := range responses {
				if res != nil {
					res := res.(SyncInitResponse)

					votes = append(votes, wavelet.SyncInitMetadata{
						User:        peers[i],
						ViewID:      res.latestViewID,
						ChunkHashes: res.chunkHashes,
					})
				}
			}

			evt.Result <- votes
		}()
	}
}

func (b *block) broadcastSyncMissingTXs(ledger *wavelet.Ledger, node *noise.Node, peer *noise.Peer) {
	for evt := range ledger.SyncTxOut {
		func() {
			peers, err := selectPeers(node, sys.SnowballSyncK)
			if err != nil {
				return
			}

			responses, err := broadcast(node, peers, SyncMissingTxRequest{ids: evt.IDs}, b.opcodeSyncMissingTxResponse)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while sending request for missing transactions")
				return
			}

			set := make(map[common.TransactionID]wavelet.Transaction)

			for _, res := range responses {
				if res != nil {
					res := res.(SyncMissingTxResponse)

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
		}()
	}
}

func (b *block) broadcastSyncDiffRequests(ledger *wavelet.Ledger, node *noise.Node, peer *noise.Peer) {
	for evt := range ledger.SyncDiffOut {
		func() {
			collected := make([][]byte, len(evt.Sources))
			var count atomic.Uint32

			var wg sync.WaitGroup
			wg.Add(len(evt.Sources))

			for chunkID, src := range evt.Sources {
				src := src

				// FIXME(zhy0919): Noise does not currently support concurrent
				//  requests/responses on a single peer.
				func() {
					defer wg.Done()

					for i := 0; i < 5; i++ {
						peerID := src.Peers[rand.Intn(len(src.Peers))]

						peer := protocol.Peer(node, peerID)
						if peer == nil {
							continue
						}

						err := peer.SendMessage(SyncChunkRequest{chunkHash: src.Hash})
						if err != nil {
							continue
						}

						var res SyncChunkResponse

						select {
						case msg := <-peer.Receive(b.opcodeSyncChunkResponse):
							res = msg.(SyncChunkResponse)
						case <-time.After(sys.QueryTimeout):
							continue
						}

						if res.diff == nil {
							continue
						}

						if remoteHash := blake2b.Sum256(res.diff); remoteHash != src.Hash {
							continue
						}

						collected[chunkID] = res.diff
						count.Add(1)

						break
					}
				}()
			}

			wg.Wait()

			if int(count.Load()) != len(evt.Sources) {
				evt.Error <- errors.New("failed to fetch some chunks from our peers")
				return
			}

			evt.Result <- collected
		}()
	}
}

func handleQueryRequest(ledger *wavelet.Ledger, peer *noise.Peer, req QueryRequest) {
	var res QueryResponse
	defer func() {
		if err := <-peer.SendMessageAsync(res); err != nil {
			fmt.Println(err)
		}
	}()

	evt := wavelet.EventIncomingQuery{TX: *req.tx, Response: make(chan *wavelet.Transaction, 1), Error: make(chan error, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending query request to ledger")
	case ledger.QueryIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting query result from ledger")
	case err := <-evt.Error:
		if err != nil {
			fmt.Printf("got an error processing query request from %s: %s\n", peer.RemoteIP().String()+":"+strconv.FormatUint(uint64(peer.RemotePort()), 10), err)
		}
	case res.preferred = <-evt.Response:
	}
}

func handleGossipRequest(ledger *wavelet.Ledger, peer *noise.Peer, req GossipRequest) {
	var res GossipResponse
	defer func() {
		if err := <-peer.SendMessageAsync(res); err != nil {
			fmt.Println(err)
		}
	}()

	evt := wavelet.EventIncomingGossip{TX: *req.TX, Vote: make(chan error, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending gossip request to ledger")
		return
	case ledger.GossipIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting vote from ledger")
		return
	case err := <-evt.Vote:
		res.vote = err == nil

		if err != nil {
			fmt.Printf("got an error processing gossip request from %s: %s\n", peer.RemoteIP().String()+":"+strconv.FormatUint(uint64(peer.RemotePort()), 10), err)
		}
	}
}

func handleOutOfSyncCheck(ledger *wavelet.Ledger, peer *noise.Peer, req SyncViewRequest) {
	var res SyncViewResponse
	defer func() {
		if err := <-peer.SendMessageAsync(res); err != nil {
			fmt.Println(err)
		}
	}()

	evt := wavelet.EventIncomingOutOfSyncCheck{Root: *req.root, Response: make(chan *wavelet.Transaction, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending out of sync check to ledger")
		return
	case ledger.OutOfSyncIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting sync check results from ledger")
		return
	case root := <-evt.Response:
		res.root = root
	}
}

func handleSyncInits(ledger *wavelet.Ledger, peer *noise.Peer, req SyncInitRequest) {
	var res SyncInitResponse
	defer func() {
		if err := <-peer.SendMessageAsync(res); err != nil {
			fmt.Println(err)
		}
	}()

	evt := wavelet.EventIncomingSyncInit{ViewID: req.viewID, Response: make(chan wavelet.SyncInitMetadata, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending sync init request to ledger")
		return
	case ledger.SyncInitIn <- evt:
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

func handleSyncChunks(ledger *wavelet.Ledger, peer *noise.Peer, req SyncChunkRequest) {
	var res SyncChunkResponse
	defer func() {
		if err := <-peer.SendMessageAsync(res); err != nil {
			fmt.Println(err)
		}
	}()

	evt := wavelet.EventIncomingSyncDiff{ChunkHash: req.chunkHash, Response: make(chan []byte, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending sync diff request to ledger")
		return
	case ledger.SyncDiffIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting sync diff results from ledger")
		return
	case chunk := <-evt.Response:
		res.diff = chunk
	}
}

func handleSyncMissingTXs(ledger *wavelet.Ledger, peer *noise.Peer, req SyncMissingTxRequest) {
	var res SyncMissingTxResponse
	defer func() {
		if err := <-peer.SendMessageAsync(res); err != nil {
			fmt.Println(err)
		}
	}()

	evt := wavelet.EventIncomingSyncTX{IDs: req.ids, Response: make(chan []wavelet.Transaction, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending missing tx sync request to ledger")
		return
	case ledger.SyncTxIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting missing tx sync results from ledger")
		return
	case txs := <-evt.Response:
		res.transactions = txs
	}
}

func (b *block) OnEnd(p *protocol.Protocol, peer *noise.Peer) error {
	return nil
}

func WaitUntilAuthenticated(peer *noise.Peer) {
	<-peer.LoadOrStore(keyAuthChannel, make(chan struct{})).(chan struct{})
}

func Ledger(node *noise.Node) *wavelet.Ledger {
	return node.Get(keyLedger).(*wavelet.Ledger)
}
