package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

var _ protocol.Block = (*block)(nil)

const (
	keyLedger      = "wavelet.ledger"
	keyBroadcaster = "wavelet.broadcaster"
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
	ledger.RegisterProcessor(sys.TagContract, new(wavelet.ContractProcessor))
	ledger.RegisterProcessor(sys.TagStake, new(wavelet.StakeProcessor))

	node.Set(keyLedger, ledger)

	broadcaster := newBroadcaster(node)
	broadcaster.Init()

	node.Set(keyBroadcaster, broadcaster)
}

func (b *block) OnBegin(p *protocol.Protocol, peer *noise.Peer) error {
	go b.receiveLoop(Ledger(peer.Node()), peer)

	close(peer.LoadOrStore(keyAuthChannel, make(chan struct{})).(chan struct{}))

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

func handleQueryRequest(ledger *wavelet.Ledger, peer *noise.Peer, req QueryRequest) {
	res := new(QueryResponse)

	vote := ledger.ReceiveTransaction(req.tx)

	res.vote = errors.Cause(vote) == wavelet.VoteAccepted
	copy(res.id[:], peer.Node().Keys.PublicKey())

	if res.vote {
		log.Consensus("vote").Debug().Hex("tx_id", req.tx.ID[:]).Msg("Gave a positive vote to a transaction.")
	} else {
		log.Consensus("vote").Debug().Hex("tx_id", req.tx.ID[:]).Err(vote).Msg("Gave a positive vote to a transaction.")
	}

	signature, err := eddsa.Sign(peer.Node().Keys.PrivateKey(), res.Write())
	if err != nil {
		log.Node().Fatal().Msg("Failed to sign query response.")
	}

	copy(res.signature[:], signature)

	_ = peer.SendMessageAsync(res)
}

func (b *block) OnEnd(p *protocol.Protocol, peer *noise.Peer) error {
	return nil
}

func WaitUntilAuthenticated(peer *noise.Peer) {
	<-peer.LoadOrStore(keyAuthChannel, make(chan struct{})).(chan struct{})
}

func BroadcastTransaction(node *noise.Node, tx *wavelet.Transaction) error {
	return Broadcaster(node).Broadcast(tx)
}

func Ledger(node *noise.Node) *wavelet.Ledger {
	return node.Get(keyLedger).(*wavelet.Ledger)
}

func Broadcaster(node *noise.Node) *broadcaster {
	return node.Get(keyBroadcaster).(*broadcaster)
}
