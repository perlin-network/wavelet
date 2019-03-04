package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
)

var _ protocol.Block = (*block)(nil)

const (
	keyLedger      = "wavelet.ledger"
	keyBroadcaster = "wavelet.broadcaster"
	keySyncer      = "wavelet.syncer"
	keyAuthChannel = "wavelet.auth.ch"
)

type block struct {
	opcodeGossipRequest  noise.Opcode
	opcodeGossipResponse noise.Opcode

	opcodeQueryRequest  noise.Opcode
	opcodeQueryResponse noise.Opcode

	opcodeSyncViewRequest  noise.Opcode
	opcodeSyncViewResponse noise.Opcode
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

	genesisPath := "config/genesis.json"

	kv := store.NewInmem()

	ledger := wavelet.NewLedger(kv, genesisPath)
	ledger.RegisterProcessor(sys.TagNop, new(wavelet.NopProcessor))
	ledger.RegisterProcessor(sys.TagTransfer, new(wavelet.TransferProcessor))
	ledger.RegisterProcessor(sys.TagContract, new(wavelet.ContractProcessor))
	ledger.RegisterProcessor(sys.TagStake, new(wavelet.StakeProcessor))

	node.Set(keyLedger, ledger)

	broadcaster := newBroadcaster(node)
	broadcaster.init()

	node.Set(keyBroadcaster, broadcaster)

	syncer := newSyncer(node)
	syncer.init()
}

func (b *block) OnBegin(p *protocol.Protocol, peer *noise.Peer) error {
	go b.receiveLoop(Ledger(peer.Node()), peer)

	close(peer.LoadOrStore(keyAuthChannel, make(chan struct{})).(chan struct{}))

	return nil
}

func (b *block) receiveLoop(ledger *wavelet.Ledger, peer *noise.Peer) {
	for {
		select {
		case req := <-peer.Receive(b.opcodeGossipRequest):
			handleGossipRequest(ledger, peer, req.(GossipRequest))
		case req := <-peer.Receive(b.opcodeQueryRequest):
			handleQueryRequest(ledger, peer, req.(QueryRequest))
		case req := <-peer.Receive(b.opcodeSyncViewRequest):
			handleSyncViewRequest(ledger, peer, req.(SyncViewRequest))
		}
	}
}

func handleSyncViewRequest(ledger *wavelet.Ledger, peer *noise.Peer, req SyncViewRequest) {
	syncer := Syncer(peer.Node())

	// TODO(kenta): add additional checks to see if the incoming root is valid
	if preferred := syncer.resolver.Preferred(); ledger.ViewID() < req.root.ViewID && preferred == nil {
		syncer.resolver.Prefer(req.root)
	}

	res := new(SyncViewResponse)
	if preferred := syncer.resolver.Preferred(); preferred == nil {
		res.root = *ledger.Root()
	} else {
		res.root = preferred.(wavelet.Transaction)
	}

	_ = <-peer.SendMessageAsync(res)
}

func handleQueryRequest(ledger *wavelet.Ledger, peer *noise.Peer, req QueryRequest) {
	// Only verify the transaction if it is a critical transaction.
	if req.tx.IsCritical(ledger.Difficulty()) {
		_ = ledger.ReceiveTransaction(req.tx)
	}

	res := new(QueryResponse)

	if req.tx.ViewID == ledger.ViewID()-1 {
		res.preferred = ledger.Root().ID
	} else {
		res.preferred = ledger.Resolver().Preferred().(common.TransactionID)
	}

	logger := log.Consensus("query")
	logger.Debug().
		Hex("preferred_id", res.preferred[:]).
		Uint64("view_id", ledger.ViewID()).
		Msg("Responded to finality query with our preferred transaction.")

	_ = <-peer.SendMessageAsync(res)
}

func handleGossipRequest(ledger *wavelet.Ledger, peer *noise.Peer, req GossipRequest) {
	res := new(GossipResponse)

	vote := ledger.ReceiveTransaction(req.tx)
	res.vote = errors.Cause(vote) == wavelet.VoteAccepted

	if logger := log.Consensus("vote"); res.vote {
		logger.Debug().Hex("tx_id", req.tx.ID[:]).Msg("Gave a positive vote to a transaction.")
	} else {
		logger.Warn().Hex("tx_id", req.tx.ID[:]).Err(vote).Msg("Gave a negative vote to a transaction.")
	}

	_ = <-peer.SendMessageAsync(res)
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

func Syncer(node *noise.Node) *syncer {
	return node.Get(keySyncer).(*syncer)
}
