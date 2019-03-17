package node

import (
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"time"
)

var _ protocol.Block = (*block)(nil)

const (
	keyLedger      = "wavelet.ledger"
	keySyncer      = "wavelet.syncer"
	keyAuthChannel = "wavelet.auth.ch"

	syncChunkSize = 1048576 // change this to a smaller value for debugging.
)

type block struct {
	opcodeGossipRequest  noise.Opcode
	opcodeGossipResponse noise.Opcode

	opcodeQueryRequest  noise.Opcode
	opcodeQueryResponse noise.Opcode

	opcodeSyncViewRequest  noise.Opcode
	opcodeSyncViewResponse noise.Opcode

	opcodeSyncDiffMetadataRequest  noise.Opcode
	opcodeSyncDiffMetadataResponse noise.Opcode
	opcodeSyncDiffChunkRequest     noise.Opcode
	opcodeSyncDiffChunkResponse    noise.Opcode

	opcodeSyncTransactionRequest  noise.Opcode
	opcodeSyncTransactionResponse noise.Opcode
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
	b.opcodeSyncDiffMetadataRequest = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncDiffMetadataRequest)(nil))
	b.opcodeSyncDiffMetadataResponse = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncDiffMetadataResponse)(nil))
	b.opcodeSyncDiffChunkRequest = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncDiffChunkRequest)(nil))
	b.opcodeSyncDiffChunkResponse = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncDiffChunkResponse)(nil))
	b.opcodeSyncTransactionRequest = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncTransactionRequest)(nil))
	b.opcodeSyncTransactionResponse = noise.RegisterMessage(noise.NextAvailableOpcode(), (*SyncTransactionResponse)(nil))

	kv := store.NewInmem()

	ledger := wavelet.NewLedger(node.Keys, kv)
	node.Set(keyLedger, ledger)

	go wavelet.Run(ledger)
}

func (b *block) OnBegin(p *protocol.Protocol, peer *noise.Peer) error {
	go b.sendLoop(Ledger(peer.Node()), peer)
	go b.receiveLoop(Ledger(peer.Node()), peer)

	close(peer.LoadOrStore(keyAuthChannel, make(chan struct{})).(chan struct{}))

	return nil
}

func (b *block) sendLoop(ledger *wavelet.Ledger, peer *noise.Peer) {
	go b.broadcastGossip(ledger, peer.Node(), peer)
	go b.broadcastQueries(ledger, peer.Node(), peer)
}

func (b *block) receiveLoop(ledger *wavelet.Ledger, peer *noise.Peer) {
	for {
		select {
		case msg := <-peer.Receive(b.opcodeGossipRequest):
			go handleGossipRequest(ledger, peer, msg.(GossipRequest))
		case req := <-peer.Receive(b.opcodeQueryRequest):
			go handleQueryRequest(ledger, peer, req.(QueryRequest))
		}
	}
}

func (b *block) broadcastGossip(ledger *wavelet.Ledger, node *noise.Node, peer *noise.Peer) {
	for evt := range ledger.GossipOut {
		func() {
			defer close(evt.Result)
			defer close(evt.Error)

			peers, err := selectPeers(node, sys.SnowballQueryK)
			if err != nil {
				fmt.Println("failed to select peers while gossiping:", err)
				evt.Error <- errors.Wrap(err, "failed to select peers while gossiping")
				return
			}

			responses, err := broadcast(node, peers, GossipRequest{TX: &evt.TX}, b.opcodeGossipResponse)
			if err != nil {
				fmt.Println("got an error gossiping:", err)
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
			defer close(evt.Result)
			defer close(evt.Error)

			peers, err := selectPeers(node, sys.SnowballQueryK)
			if err != nil {
				fmt.Println("failed to select peers while querying:", err)
				evt.Error <- errors.Wrap(err, "failed to select peers while querying")
				return
			}

			responses, err := broadcast(node, peers, QueryRequest{tx: &evt.TX}, b.opcodeQueryResponse)
			if err != nil {
				fmt.Println("got an error querying:", err)
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

func handleQueryRequest(ledger *wavelet.Ledger, peer *noise.Peer, req QueryRequest) {
	var res QueryResponse
	defer func() {
		if err := <-peer.SendMessageAsync(res); err != nil {
			fmt.Println(err)
		}
	}()

	evt := wavelet.EventIncomingQuery{TX: *req.tx, Response: make(chan *wavelet.Transaction, 1), Error: make(chan error, 1)}

	select {
	case <-time.After(3 * time.Second):
		fmt.Println("timed out sending query request to ledger")
	case ledger.QueryIn <- evt:
	}

	select {
	case <-time.After(3 * time.Second):
		fmt.Println("timed out getting query result from ledger")
	case err := <-evt.Error:
		fmt.Println("got an error processing query request:", err)
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
	case <-time.After(3 * time.Second):
		fmt.Println("timed out sending gossip request to ledger")
	case ledger.GossipIn <- evt:
	}

	select {
	case <-time.After(3 * time.Second):
		fmt.Println("timed out getting vote from ledger")
	case err := <-evt.Vote:
		res.vote = err == nil

		if err != nil {
			fmt.Println(err)
		}
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
