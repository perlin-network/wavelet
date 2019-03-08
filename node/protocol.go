package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/store"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
)

var _ protocol.Block = (*block)(nil)

const (
	keyLedger      = "wavelet.ledger"
	keyBroadcaster = "wavelet.broadcaster"
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

	node.Set(keySyncer, syncer)
}

func (b *block) OnBegin(p *protocol.Protocol, peer *noise.Peer) error {
	go b.receiveLoop(Ledger(peer.Node()), peer)

	close(peer.LoadOrStore(keyAuthChannel, make(chan struct{})).(chan struct{}))

	return nil
}

func (b *block) receiveLoop(ledger *wavelet.Ledger, peer *noise.Peer) {
	chunkCache := newLRU(1024) // 1024 * 4MB

	for {
		select {
		case req := <-peer.Receive(b.opcodeGossipRequest):
			handleGossipRequest(ledger, peer, req.(GossipRequest))
		case req := <-peer.Receive(b.opcodeQueryRequest):
			handleQueryRequest(ledger, peer, req.(QueryRequest))
		case req := <-peer.Receive(b.opcodeSyncViewRequest):
			handleSyncViewRequest(ledger, peer, req.(SyncViewRequest))
		case req := <-peer.Receive(b.opcodeSyncDiffMetadataRequest):
			handleSyncDiffMetadataRequest(ledger, peer, req.(SyncDiffMetadataRequest), chunkCache)
		case req := <-peer.Receive(b.opcodeSyncDiffChunkRequest):
			handleSyncDiffChunkRequest(ledger, peer, req.(SyncDiffChunkRequest), chunkCache)
		}
	}
}

func handleSyncDiffMetadataRequest(ledger *wavelet.Ledger, peer *noise.Peer, req SyncDiffMetadataRequest, chunkCache *lru) {
	diff := ledger.DumpDiff(req.viewID)

	var chunkHashes [][blake2b.Size256]byte

	for i := 0; i < len(diff); i += syncChunkSize {
		end := i + syncChunkSize

		if end > len(diff) {
			end = len(diff)
		}

		hash := blake2b.Sum256(diff[i:end])

		chunkCache.put(hash, diff[i:end])
		chunkHashes = append(chunkHashes, hash)
	}

	if err := <-peer.SendMessageAsync(&SyncDiffMetadataResponse{
		latestViewID: ledger.ViewID(),
		chunkHashes:  chunkHashes,
	}); err != nil {
		_ = peer.DisconnectAsync()
	}
}

func handleSyncDiffChunkRequest(ledger *wavelet.Ledger, peer *noise.Peer, req SyncDiffChunkRequest, chunkCache *lru) {
	var res SyncDiffChunkResponse

	if chunk, found := chunkCache.load(req.chunkHash); found {
		res.diff = chunk.([]byte)
		res.found = true

		providedHash := blake2b.Sum256(res.diff)

		logger := log.Node()
		logger.Info().
			Hex("requested_hash", req.chunkHash[:]).
			Hex("provided_hash", providedHash[:]).
			Msg("Responded to sync chunk request.")
	}

	if err := <-peer.SendMessageAsync(&res); err != nil {
		_ = peer.DisconnectAsync()
	}
}

func handleSyncViewRequest(ledger *wavelet.Ledger, peer *noise.Peer, req SyncViewRequest) {
	res := new(SyncViewResponse)

	if req.root == nil {
		return
	}

	syncer := Syncer(peer.Node())
	syncer.recordRootFromAccount(protocol.PeerID(peer), req.root)

	if err := wavelet.AssertValidTransaction(req.root); err == nil {
		preferred := syncer.resolver.Preferred()

		if ledger.ViewID() < req.root.ViewID && preferred == nil {
			syncer.resolver.Prefer(req.root)
		}
	}

	if preferred := syncer.resolver.Preferred(); preferred == nil {
		res.root = ledger.Root()
	} else {
		res.root = preferred.(*wavelet.Transaction)
	}

	if err := <-peer.SendMessageAsync(res); err != nil {
		_ = peer.DisconnectAsync()
	}
}

func handleQueryRequest(ledger *wavelet.Ledger, peer *noise.Peer, req QueryRequest) {
	res := new(QueryResponse)
	defer func() {
		if err := <-peer.SendMessageAsync(res); err != nil {
			_ = peer.DisconnectAsync()
		}
	}()

	if req.tx == nil {
		return
	}

	if Broadcaster(peer.Node()).Paused.Load() {
		return
	}

	// If our node does not prefer any critical transaction yet, set a critical
	// transaction to initially prefer.
	if err := wavelet.AssertValidTransaction(req.tx); err == nil {
		preferred := ledger.Resolver().Preferred()

		if req.tx.IsCritical(ledger.Difficulty()) {
			correctViewID := ledger.AssertValidViewID(req.tx) == nil
			preferredNotSet := preferred == nil && req.tx.ID != ledger.Root().ID

			if correctViewID && preferredNotSet {
				ledger.Resolver().Prefer(req.tx)
			}
		}
	}

	if req.tx.ViewID == ledger.ViewID()-1 {
		res.preferred = ledger.Root()
	} else if preferred := ledger.Resolver().Preferred(); preferred != nil {
		res.preferred = preferred.(*wavelet.Transaction)
	}

	if res.preferred != nil {
		logger := log.Consensus("query")
		logger.Debug().
			Hex("preferred_id", res.preferred.ID[:]).
			Uint64("view_id", ledger.ViewID()).
			Msg("Responded to finality query with our preferred transaction.")
	}
}

func handleGossipRequest(ledger *wavelet.Ledger, peer *noise.Peer, req GossipRequest) {
	res := new(GossipResponse)
	defer func() {
		if err := <-peer.SendMessageAsync(res); err != nil {
			_ = peer.DisconnectAsync()
		}
	}()

	if req.tx == nil {
		return
	}

	if Broadcaster(peer.Node()).Paused.Load() {
		return
	}

	_, seen := ledger.FindTransaction(req.tx.ID)

	vote := ledger.ReceiveTransaction(req.tx)
	res.vote = errors.Cause(vote) == wavelet.VoteAccepted

	if logger := log.Consensus("vote"); res.vote {
		logger.Debug().Hex("tx_id", req.tx.ID[:]).Msg("Gave a positive vote to a transaction.")
	} else {
		logger.Warn().Hex("tx_id", req.tx.ID[:]).Err(vote).Msg("Gave a negative vote to a transaction.")
	}

	// Gossip transaction out to our peers just once. Ignore any errors if gossiping out fails.
	if !seen && res.vote {
		go func() {
			_ = BroadcastTransaction(peer.Node(), req.tx)
		}()
	}
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
