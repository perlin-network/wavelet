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
	keyLedger      = "wavelet.ledger"
	keyAuthChannel = "wavelet.auth.ch"
)

type block struct {
	opcodeQueryRequest  noise.Opcode
	opcodeQueryResponse noise.Opcode
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

	go gossipLoop(ledger, peer)
	go b.receiveLoop(ledger, peer)

	close(peer.LoadOrStore(keyAuthChannel, make(chan struct{})).(chan struct{}))

	return nil
}

func WaitUntilAuthenticated(peer *noise.Peer) {
	<-peer.LoadOrStore(keyAuthChannel, make(chan struct{})).(chan struct{})
}

func (b *block) receiveLoop(ledger *wavelet.Ledger, peer *noise.Peer) {
	for {
		select {
		case req := <-peer.Receive(b.opcodeQueryRequest):
			handleQueryRequest(ledger, peer, req.(QueryRequest))
		}
	}
}

func handleQueryRequest(ledger *wavelet.Ledger, peer *noise.Peer, req QueryRequest) {
	res := new(QueryResponse)

	seen := ledger.HasTransactionInView(req.tx.ID)

	if !seen {
		vote := ledger.ReceiveTransaction(req.tx)

		res.vote = errors.Cause(vote) == wavelet.VoteAccepted
		copy(res.id[:], peer.Node().Keys.PublicKey())

		if res.vote {
			log.Debug().Msgf("Gave a positive vote to transaction %x.", req.tx.ID)
		} else {
			log.Debug().Err(vote).Msgf("Gave a negative vote to transaction %x.", req.tx.ID)
		}
	} else {
		res.vote = true
	}

	signature, err := eddsa.Sign(peer.Node().Keys.PrivateKey(), res.Write())
	if err != nil {
		panic("wavelet: failed to sign query response")
	}

	copy(res.signature[:], signature)

	err = peer.SendMessage(res)
	if err != nil {
		log.Warn().Err(err).Msg("Failed to send back query response.")
	}

	// Gossip out positively-voted critical transactions.
	if res.vote && !seen && req.tx.IsCritical(ledger.Difficulty()) {
		gossipOutTransaction(peer.Node(), req.tx)
	}
}

func (b *block) OnEnd(p *protocol.Protocol, peer *noise.Peer) error {
	return nil
}

func BroadcastTransaction(node *noise.Node, tx *wavelet.Transaction) error {
	ledger := Ledger(node)

	vote := ledger.ReceiveTransaction(tx)
	switch errors.Cause(vote) {
	case wavelet.VoteAccepted:
	case wavelet.VoteRejected:
		return errors.Wrap(vote, "broadcast: could not place newly created transaction to broadcast into view graph")
	}

	err := QueryTransaction(node, tx)
	if err != nil {
		return errors.Wrap(err, "broadcast: failed to gossip out transaction")
	}

	// If the transaction was critical, then we safely don't have to
	// send out any nops.
	//
	// Otherwise, contribute to the network by sending out nops to get
	// this nodes transaction accepted.
	if tx.IsCritical(ledger.Difficulty()) {
		gossipOutTransaction(node, tx)
		return nil
	}

	for {
		time.Sleep(1 * time.Millisecond)

		nop, err := ledger.NewTransaction(node.Keys, sys.TagNop, nil)
		if err != nil {
			return errors.Wrap(err, "broadcast: failed to create nop")
		}

		vote = ledger.ReceiveTransaction(nop)
		switch errors.Cause(vote) {
		case wavelet.VoteAccepted:
		case wavelet.VoteRejected:
			return errors.Wrap(vote, "broadcast: could not place newly created nop to broadcast into view graph")
		}

		err = QueryTransaction(node, nop)
		if err != nil {
			return errors.Wrap(err, "broadcast: failed to gossip out transaction")
		}

		if nop.IsCritical(ledger.Difficulty()) {
			gossipOutTransaction(node, nop)
			return nil
		}
	}
}

func QueryTransaction(node *noise.Node, tx *wavelet.Transaction) error {
	ledger := Ledger(node)

	peerIDs := skademlia.FindClosestPeers(skademlia.Table(node), protocol.NodeID(node).Hash(), sys.SnowballK)

	if len(peerIDs) < sys.SnowballK {
		return errors.Errorf("broadcast: only connected to %d peer(s), but consensus requires being connected to at a minimum of %d peer(s). please connect to more peer(s).", len(peerIDs), sys.SnowballK)
	}

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
			case <-time.After(sys.QueryTimeout):
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

func Ledger(node *noise.Node) *wavelet.Ledger {
	return node.Get(keyLedger).(*wavelet.Ledger)
}
