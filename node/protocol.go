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
	"runtime"
	"sync"
	"time"
)

const (
	SignalAuthenticated = "wavelet.auth.ch"
)

type Protocol struct {
	opcodeGossip        byte
	opcodeQuery         byte
	opcodeOutOfSync     byte
	opcodeSyncInit      byte
	opcodeDownloadChunk byte
	opcodeDownloadTx    byte

	ledger *wavelet.Ledger

	network *skademlia.Protocol
	keys    *skademlia.Keypair

	broadcaster *broadcaster

	ctx    context.Context
	cancel func()
	wg     sync.WaitGroup
}

func New(network *skademlia.Protocol, keys *skademlia.Keypair, kv store.KV, genesis *string) *Protocol {
	return &Protocol{ledger: wavelet.NewLedger(keys, kv, genesis), network: network, keys: keys}
}

func (p *Protocol) Stop() {
	p.ledger.Stop()
	p.cancel()
	p.wg.Wait()

	p.broadcaster.Stop()
}

func (p *Protocol) Ledger() *wavelet.Ledger {
	return p.ledger
}

func (p *Protocol) Network() *skademlia.Protocol {
	return p.network
}

func (p *Protocol) RegisterOpcodes(node *noise.Node) {
	p.opcodeGossip = node.Handle(node.NextAvailableOpcode(), p.handleGossip)
	p.opcodeQuery = node.Handle(node.NextAvailableOpcode(), p.handleQuery)
	p.opcodeOutOfSync = node.Handle(node.NextAvailableOpcode(), p.handleOutOfSyncRequest)
	p.opcodeSyncInit = node.Handle(node.NextAvailableOpcode(), p.handleSyncInit)
	p.opcodeDownloadChunk = node.Handle(node.NextAvailableOpcode(), p.handleDownloadChunk)
	p.opcodeDownloadTx = node.Handle(node.NextAvailableOpcode(), p.handleDownloadTx)
}

func (p *Protocol) Init(node *noise.Node) {
	p.ctx, p.cancel = context.WithCancel(context.Background())

	p.broadcaster = NewBroadcaster(runtime.NumCPU()*2, 1024)

	go p.sendLoop(node)
	go p.ledger.Run()
}

func (p *Protocol) Protocol() noise.ProtocolBlock {
	return func(ctx noise.Context) error {
		rid := ctx.Get(skademlia.KeyID)

		if rid == nil {
			return errors.New("wavelet: user does not have a s/kademlia id registered")
		}

		id := rid.(*skademlia.ID)

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
	go p.broadcastQuery(node)
	go p.broadcastOutOfSync(node)
	go p.broadcastSyncInit(node)
	go p.broadcastDownloadChunk(node)
	go p.broadcastDownloadTx(node)
}

func (p *Protocol) broadcastGossip(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.GossipTxOut:
			peers, err := SelectPeers(p.network.Peers(node), sys.SnowballK)
			if err != nil {
				continue
			}

			p.broadcaster.Broadcast(
				p.ctx,
				peers,
				p.opcodeGossip,
				evt.TX.Marshal(),
				false,
			)
		}
	}
}

func (p *Protocol) broadcastQuery(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.QueryOut:
			peers, err := SelectPeers(p.network.Peers(node), sys.SnowballK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "failed to select peers while querying")
				continue
			}

			//before := time.Now()

			responses := p.broadcaster.Broadcast(
				p.ctx,
				peers,
				p.opcodeQuery,
				QueryRequest{round: *evt.Round}.Marshal(),
				true,
			)

			//fmt.Printf("query took %s seconds\n", time.Now().Sub(before).String())

			votes := make([]wavelet.VoteQuery, len(responses))

			for i, buf := range responses {
				id := peers[i].Ctx().Get(skademlia.KeyID)

				if id == nil {
					continue
				}

				if buf != nil {
					res, err := UnmarshalQueryResponse(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("error while unmarshaling query response", err)
						continue
					}

					votes[i].Preferred = res.preferred
				}

				votes[i].Voter = id.(*skademlia.ID).PublicKey()
			}

			evt.Result <- votes
		}
	}
}

func (p *Protocol) broadcastOutOfSync(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.OutOfSyncOut:
			peers, err := SelectPeers(p.network.Peers(node), sys.SnowballK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while selecting peers for out of sync check")
				continue
			}

			responses := p.broadcaster.Broadcast(
				p.ctx,
				peers,
				p.opcodeOutOfSync,
				nil,
				true,
			)

			votes := make([]wavelet.VoteOutOfSync, len(peers))

			for i, buf := range responses {
				id := peers[i].Ctx().Get(skademlia.KeyID)

				if id == nil {
					continue
				}

				if buf != nil {
					res, err := UnmarshalOutOfSyncResponse(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("Error while unmarshaling out of sync response", err)
						continue
					}

					votes[i].Round = res.round
				}

				votes[i].Voter = id.(*skademlia.ID).PublicKey()
			}

			evt.Result <- votes

		}
	}
}

func (p *Protocol) broadcastSyncInit(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.SyncInitOut:
			peers, err := SelectPeers(p.network.Peers(node), sys.SnowballK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while selecting peers for sync init")
				continue
			}

			responses := p.broadcaster.Broadcast(
				p.ctx,
				peers,
				p.opcodeSyncInit,
				SyncInitRequest{viewID: evt.RoundID}.Marshal(),
				true,
			)

			votes := make([]wavelet.SyncInitMetadata, len(responses))

			for i, buf := range responses {
				id := peers[i].Ctx().Get(skademlia.KeyID)

				if id == nil {
					continue
				}

				if buf != nil {
					res, err := UnmarshalSyncInitResponse(bytes.NewReader(buf))

					if err != nil {
						fmt.Println("Error while unmarshaling sync init response", err)
						continue
					}

					votes[i].RoundID = res.latestViewID
					votes[i].ChunkHashes = res.chunkHashes
				}

				votes[i].PeerID = id.(*skademlia.ID)
			}

			evt.Result <- votes
		}
	}
}

func (p *Protocol) broadcastDownloadTx(node *noise.Node) {
	p.wg.Add(1)
	defer p.wg.Done()

	for {
		select {
		case <-p.ctx.Done():
			return
		case evt := <-p.ledger.DownloadTxOut:
			peers, err := SelectPeers(p.network.Peers(node), sys.SnowballK)
			if err != nil {
				evt.Error <- errors.Wrap(err, "got an error while selecting peers for syncing missing transactions")
				continue
			}

			responses := p.broadcaster.Broadcast(
				p.ctx,
				peers,
				p.opcodeDownloadTx,
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

func (p *Protocol) broadcastDownloadChunk(node *noise.Node) {
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
							buf, err := peer.Request(p.opcodeDownloadChunk, SyncChunkRequest{chunkHash: src.Hash}.Marshal())
							if err != nil {
								return nil, err
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

func (p *Protocol) handleQuery(ctx noise.Context, buf []byte) ([]byte, error) {
	var res QueryResponse

	req, err := UnmarshalQueryRequest(bytes.NewReader(buf))
	if err != nil {
		return res.Marshal(), errors.Wrap(err, "error while unmarshaling query request")
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
			fmt.Printf("got an error processing query request from %s: %s\n", ctx.Peer().Addr(), err)
		}
	case preferred := <-evt.Response:
		if preferred != nil {
			res.preferred = *preferred
		}
	}

	return res.Marshal(), nil
}

func (p *Protocol) handleOutOfSyncRequest(ctx noise.Context, buf []byte) ([]byte, error) {
	var res OutOfSyncResponse

	evt := wavelet.EventIncomingOutOfSyncCheck{Response: make(chan *wavelet.Round, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending out of sync check to ledger")
		return res.Marshal(), nil
	case p.ledger.OutOfSyncIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting out of sync check results from ledger")
		return res.Marshal(), nil
	case root := <-evt.Response:
		res.round = *root
	}

	return res.Marshal(), nil
}

func (p *Protocol) handleSyncInit(ctx noise.Context, buf []byte) ([]byte, error) {
	var res SyncInitResponse

	req, err := UnmarshalSyncInitRequest(bytes.NewReader(buf))
	if err != nil {
		return res.Marshal(), errors.Wrap(err, "error while unmarshaling sync init request")
	}

	evt := wavelet.EventIncomingSyncInit{RoundID: req.viewID, Response: make(chan wavelet.SyncInitMetadata, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending sync init request to ledger")
		return res.Marshal(), nil
	case p.ledger.SyncInitIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting sync init results from ledger")
		return res.Marshal(), nil
	case data := <-evt.Response:
		res.chunkHashes = data.ChunkHashes
		res.latestViewID = data.RoundID
	}

	return res.Marshal(), nil
}

func (p *Protocol) handleDownloadChunk(ctx noise.Context, buf []byte) ([]byte, error) {
	var res SyncChunkResponse

	req, err := UnmarshalSyncChunkRequest(bytes.NewReader(buf))
	if err != nil {
		return res.Marshal(), errors.Wrap(err, "error while unmarshaling sync chunk request")
	}

	evt := wavelet.EventIncomingSyncDiff{ChunkHash: req.chunkHash, Response: make(chan []byte, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending sync diff request to ledger")
		return res.Marshal(), nil
	case p.ledger.SyncDiffIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting sync diff results from ledger")
		return res.Marshal(), nil
	case chunk := <-evt.Response:
		res.diff = chunk
	}

	return res.Marshal(), nil
}

func (p *Protocol) handleDownloadTx(ctx noise.Context, buf []byte) ([]byte, error) {
	var res DownloadTxResponse

	req, err := UnmarshalDownloadTxRequest(bytes.NewReader(buf))
	if err != nil {
		return res.Marshal(), errors.Wrap(err, "error while unmarshaling download tx request")
	}

	evt := wavelet.EventIncomingDownloadTX{IDs: req.ids, Response: make(chan []wavelet.Transaction, 1)}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending missing tx sync request to ledger")
		return res.Marshal(), nil
	case p.ledger.DownloadTxIn <- evt:
	}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out getting missing tx sync results from ledger")
		return res.Marshal(), nil
	case txs := <-evt.Response:
		res.transactions = txs
	}

	return res.Marshal(), nil
}

func (p *Protocol) handleGossip(ctx noise.Context, buf []byte) ([]byte, error) {
	tx, err := wavelet.UnmarshalTransaction(bytes.NewReader(buf))
	if err != nil {
		fmt.Println("error while unmarshaling gossip request", err)
		return nil, errors.Wrap(err, "error while unmarshaling gossip request")
	}

	evt := wavelet.EventGossip{TX: tx}

	select {
	case <-time.After(1 * time.Second):
		fmt.Println("timed out sending gossip request to ledger")
		return nil, errors.New("timed out sending gossip request to ledger")
	case p.ledger.GossipTxIn <- evt:
	}

	return nil, nil
}
