package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"sync"
	"time"
)

type broadcastItem struct {
	tx     *wavelet.Transaction
	result chan error
}

type broadcaster struct {
	node   *noise.Node
	ledger *wavelet.Ledger
	queue  chan broadcastItem

	broadcastingNops bool
}

func newBroadcaster(node *noise.Node) *broadcaster {
	return &broadcaster{node: node, ledger: Ledger(node), queue: make(chan broadcastItem, 1024)}
}

func (b *broadcaster) init() {
	go b.work()
}

func (b *broadcaster) Broadcast(tx *wavelet.Transaction) error {
	item := broadcastItem{tx: tx, result: make(chan error, 1)}
	b.queue <- item

	select {
	case err := <-item.result:
		return err
	case <-time.After(3 * time.Second):
		return errors.New("broadcast: timed out")
	}
}

func (b *broadcaster) work() {
	logger := log.Broadcaster()

	for {
		preferredID := b.ledger.Resolver().Preferred()

		if preferredID == common.ZeroTransactionID {
			b.gossiping(logger)
		} else {
			b.broadcastingNops = false
			b.querying(logger)
		}
	}
}

func (b *broadcaster) querying(logger zerolog.Logger) {
	preferredID := b.ledger.Resolver().Preferred()
	preferred, exists := b.ledger.FindTransaction(preferredID)

	if !exists {
		return
	}

	logger.Log().
		Bool("broadcast_nops", false).
		Hex("tx_id", preferred.ID[:]).
		Msg("Broadcasting out our preferred transaction.")

	if err := b.query(preferred); err != nil {
		logger.Warn().
			Err(err).
			Msg("Got an error while querying.")
		return
	}
}

func (b *broadcaster) gossiping(logger zerolog.Logger) {
	var item broadcastItem

	select {
	case popped := <-b.queue:
		item = popped

		logger.Log().
			Bool("broadcast_nops", b.broadcastingNops).
			Hex("tx_id", popped.tx.ID[:]).
			Msg("Broadcasting out queued transaction.")
	case <-time.After(1 * time.Millisecond):
		// If there is nothing we need to broadcast urgently, then broadcast
		// a nop (if we have previously broadcasted a transaction beforehand).
		if !b.broadcastingNops {
			return
		}

		nop, err := b.ledger.NewTransaction(b.node.Keys, sys.TagNop, nil)
		if err != nil {
			return
		}

		item = broadcastItem{tx: nop, result: nil}

		logger.Log().
			Bool("broadcast_nops", true).
			Hex("tx_id", nop.ID[:]).
			Msg("Broadcasting out nop transaction.")
	}

	if b.ledger.ViewID() != item.tx.ViewID {
		err := errors.New("broadcast: consensus round already finalized; submit transaction again")
		err = errors.Wrapf(err, "tx %x", item.tx.ID)

		if item.result != nil {
			item.result <- err
		} else {
			logger.Warn().
				Err(err).
				Msg("Stopped a broadcast during assertions.")
		}
		return
	}

	if err := b.gossip(item.tx); err != nil {
		if item.result != nil {
			item.result <- err
		} else {
			logger.Warn().
				Err(err).
				Msg("Got an error while gossiping.")
		}
		return
	}

	logger.Debug().
		Hex("tx_id", item.tx.ID[:]).
		Bool("broadcast_nops", b.broadcastingNops).
		Msg("Successfully broadcasted out transaction.")

	// Start broadcasting nops if we have successfully broadcasted
	// some arbitrary transaction.
	if !b.broadcastingNops {
		b.broadcastingNops = true

		logger.Log().
			Bool("broadcast_nops", true).
			Msg("Started broadcasting nops.")
	}

	if item.result != nil {
		item.result <- nil
	}

	// If we have advanced one view ID, stop broadcasting nops.
	if b.ledger.ViewID() != item.tx.ViewID {
		b.broadcastingNops = false

		logger.Log().
			Bool("broadcast_nops", true).
			Msg("Stopped broadcasting nops.")
	}
}

func (b *broadcaster) selectPeers(amount int) ([]protocol.ID, error) {
	peerIDs := skademlia.FindClosestPeers(skademlia.Table(b.node), protocol.NodeID(b.node).Hash(), amount)

	if len(peerIDs) < sys.SnowballK {
		return nil, errors.Errorf("broadcast: only connected to %d peer(s) "+
			"but require a minimum of %d peer(s)", len(peerIDs), sys.SnowballK)
	}

	return peerIDs, nil
}

func (b *broadcaster) broadcast(req noise.Message, res noise.Opcode) ([]common.AccountID, []noise.Message, error) {
	peerIDs, err := b.selectPeers(sys.SnowballK)
	if err != nil {
		return nil, nil, err
	}

	var accounts []common.AccountID
	var responses []noise.Message

	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(peerIDs))

	record := func(account common.AccountID, res noise.Message) {
		mu.Lock()
		defer mu.Unlock()

		accounts = append(accounts, account)
		responses = append(responses, res)
	}

	for _, peerID := range peerIDs {
		peerID := peerID

		var account common.AccountID
		copy(account[:], peerID.PublicKey())

		go func() {
			defer wg.Done()

			peer := protocol.Peer(b.node, peerID)
			if peer == nil {
				record(account, nil)
				return
			}

			// Send query request.
			err := peer.SendMessage(req)
			if err != nil {
				record(account, nil)
				return
			}

			select {
			case msg := <-peer.Receive(res):
				record(account, msg)
			case <-time.After(sys.QueryTimeout):
				record(account, nil)
			}
		}()
	}

	wg.Wait()

	return accounts, responses, nil
}

func (b *broadcaster) query(preferred *wavelet.Transaction) error {
	opcodeQueryResponse, err := noise.OpcodeFromMessage((*QueryResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "broadcast: response opcode not registered")
	}

	accounts, responses, err := b.broadcast(QueryRequest{tx: preferred}, opcodeQueryResponse)
	if err != nil {
		return err
	}

	votes := make(map[common.AccountID]common.TransactionID)
	for i, res := range responses {
		if res != nil {
			votes[accounts[i]] = res.(QueryResponse).preferred
		}
	}

	weights := b.ledger.ComputeStakeDistribution(accounts)

	counts := make(map[common.TransactionID]float64)

	for account, preferred := range votes {
		counts[preferred] += weights[account]
	}

	if err := b.ledger.ProcessQuery(counts); err != nil {
		return errors.Wrap(err, "broadcast: failed to process query results")
	}

	return nil
}

func (b *broadcaster) gossip(tx *wavelet.Transaction) error {
	opcodeGossipResponse, err := noise.OpcodeFromMessage((*GossipResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "broadcast: response opcode not registered")
	}

	accounts, responses, err := b.broadcast(GossipRequest{tx: tx}, opcodeGossipResponse)
	if err != nil {
		return err
	}

	votes := make(map[common.AccountID]bool)
	for i, res := range responses {
		if res != nil {
			votes[accounts[i]] = res.(GossipResponse).vote
		}
	}

	weights := b.ledger.ComputeStakeDistribution(accounts)

	var accum float64

	for account, vote := range votes {
		if vote {
			accum += weights[account]
		}
	}

	if accum < sys.SnowballAlpha {
		return errors.Errorf("broadcast: less than %.1f%% of queried K peers find tx %x valid", sys.SnowballAlpha*100, tx.ID)
	}

	if err := b.ledger.ReceiveTransaction(tx); err != nil && errors.Cause(err) != wavelet.VoteAccepted {
		return errors.Wrap(err, "broadcast: failed to add successfully queried tx to view-graph")
	}

	return nil
}
