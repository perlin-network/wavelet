package net

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
	"sync"
	"time"
)

var _ protocol.Block = (*block)(nil)

const (
	keyLedger        = "wavelet.ledger"
	keyGossipChannel = "wavelet.gossip.ch"
)

type block struct {
	opcodeQueryRequest  noise.Opcode
	opcodeQueryResponse noise.Opcode
}

func gossipChannel(node *noise.Node) chan interface{} {
	return node.LoadOrStore(keyGossipChannel, make(chan interface{})).(chan interface{})
}

func New() *block {
	return &block{}
}

func (b *block) OnRegister(p *protocol.Protocol, node *noise.Node) {
	b.opcodeQueryRequest = noise.RegisterMessage(noise.NextAvailableOpcode(), (*QueryRequest)(nil))
	b.opcodeQueryResponse = noise.RegisterMessage(noise.NextAvailableOpcode(), (*QueryResponse)(nil))

	genesisPath := "config/genesis.json"

	kv := store.NewInmem()

	ledger := wavelet.NewLedger(kv, genesisPath)
	ledger.RegisterProcessor(sys.TagNop, new(wavelet.NopProcessor))
	ledger.RegisterProcessor(sys.TagTransfer, new(wavelet.TransferProcessor))
	ledger.RegisterProcessor(sys.TagStake, new(wavelet.StakeProcessor))

	node.Set(keyLedger, ledger)
}

func (b *block) OnBegin(p *protocol.Protocol, peer *noise.Peer) error {
	ledger := peer.Node().Get(keyLedger).(*wavelet.Ledger)

	go b.receiveLoop(ledger, peer)
	go b.gossipLoop(ledger, peer)

	return nil
}

func (b *block) receiveLoop(ledger *wavelet.Ledger, peer *noise.Peer) {
	for {
		select {
		case req := <-peer.Receive(b.opcodeQueryRequest):
			handleQueryRequest(ledger, peer, req.(QueryRequest))
		}
	}
}

func (b *block) gossipLoop(ledger *wavelet.Ledger, peer *noise.Peer) {
	for range gossipChannel(peer.Node()) {
		// TODO(kenta): handle message gossiping here
	}
}

func handleQueryRequest(ledger *wavelet.Ledger, peer *noise.Peer, req QueryRequest) {
	vote := ledger.ReceiveTransaction(req.tx)

	res := new(QueryResponse)
	copy(res.id[:], peer.Node().Keys.PublicKey())

	switch errors.Cause(vote) {
	case wavelet.VoteAccepted:
		res.vote = true
	case wavelet.VoteRejected:
		log.Warn().Err(vote).Msg("Rejected an incoming transaction.")
	}

	signature, err := eddsa.Sign(peer.Node().Keys.PrivateKey(), res.Write())
	if err != nil {
		panic("wavelet: failed to sign query response")
	}

	copy(res.signature[:], signature)

	_ = peer.SendMessageAsync(res)
}

func (b *block) OnEnd(p *protocol.Protocol, peer *noise.Peer) error {
	return nil
}

func BroadcastTransaction(node *noise.Node, tag byte, payload []byte) error {
	ledger := node.Get(keyLedger).(*wavelet.Ledger)

	tx, err := ledger.NewTransaction(node.Keys.(*skademlia.Keypair), tag, payload)
	if err != nil {
		return errors.Wrap(err, "wavelet: failed to create transaction to broadcast")
	}

	err = ledger.ReceiveTransaction(tx)
	if err != nil {
		return errors.Wrap(err, "wavelet: could not place newly created transaction to broadcast into view graph")
	}

	return QueryTransaction(node, tx)
}

func QueryTransaction(node *noise.Node, tx *wavelet.Transaction) error {
	ledger := node.Get(keyLedger).(*wavelet.Ledger)

	peerIDs := skademlia.FindClosestPeers(skademlia.Table(node), protocol.NodeID(node).Hash(), sys.SnowballK)

	var wg sync.WaitGroup
	wg.Add(len(peerIDs))

	responses := make(map[[blake2b.Size256]byte]bool)
	var mu sync.Mutex

	recordResponse := func(rawID []byte, response bool) {
		mu.Lock()
		defer mu.Unlock()

		var id [blake2b.Size256]byte
		copy(id[:], rawID)

		responses[id] = response
	}

	opcodeQueryResponse, err := noise.OpcodeFromMessage((*QueryResponse)(nil))
	if err != nil {
		panic("wavelet: query response opcode not registered")
	}

	req := QueryRequest{tx: tx}

	for _, peerID := range peerIDs {
		peerID := peerID

		go func() {
			defer wg.Done()

			peer := protocol.Peer(node, peerID)
			if peer == nil {
				recordResponse(peerID.PublicKey(), false)
				return
			}

			// Send query request.
			err := peer.SendMessage(req)
			if err != nil {
				recordResponse(peerID.PublicKey(), false)
				return
			}

			// Receive query response.
			var res QueryResponse

			select {
			case msg := <-peer.Receive(opcodeQueryResponse):
				res = msg.(QueryResponse)
			case <-time.After(3 * time.Second):
				recordResponse(peerID.PublicKey(), false)
				return
			}

			// Verify query response signature.
			signature := res.signature
			res.signature = [wavelet.SignatureSize]byte{}

			err = eddsa.Verify(peerID.PublicKey(), res.Write(), signature[:])
			if err != nil {
				recordResponse(peerID.PublicKey(), false)
				return
			}

			recordResponse(peerID.PublicKey(), res.vote)
		}()
	}

	wg.Wait()

	err = ledger.ReceiveQuery(tx, responses)

	if errors.Cause(err) != wavelet.ErrTxNotCritical {
		return errors.Wrap(err, "wavelet: failed to handle critical tx")
	}

	return nil
}
