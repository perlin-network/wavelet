package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
	"go.uber.org/atomic"
	"time"
)

type broadcastItem struct {
	tx     *wavelet.Transaction
	result chan error
}

type broadcaster struct {
	node   *noise.Node
	ledger *wavelet.Ledger

	queue chan broadcastItem

	pause  chan struct{}
	Paused atomic.Bool

	broadcastingNops bool
}

func newBroadcaster(node *noise.Node) *broadcaster {
	return &broadcaster{
		node:             node,
		ledger:           Ledger(node),
		queue:            make(chan broadcastItem, 4096),
		pause:            make(chan struct{}),
		broadcastingNops: false,
	}
}

func (b *broadcaster) Pause() {
	b.Paused.Store(true)
	b.pause <- struct{}{}
}

func (b *broadcaster) Resume() {
	b.Paused.Store(false)
	b.init()
}

func (b *broadcaster) init() {
	go b.loop()
}

func (b *broadcaster) Broadcast(tx *wavelet.Transaction) error {
	if b.Paused.Load() {
		return errors.New("broadcast: broadcaster was paused")
	}

	item := broadcastItem{tx: tx, result: make(chan error, 1)}

	select {
	case b.queue <- item:
	default:
		return errors.New("broadcaster: queue is full")
	}

	select {
	case err, available := <-item.result:
		if !available {
			return errors.New("broadcast: broadcaster was paused")
		}

		return err
	case <-time.After(3 * time.Second):
		return errors.New("broadcast: timed out")
	}
}

func (b *broadcaster) loop() {
	logger := log.Broadcaster()

	for {
		select {
		case <-b.pause: // Empty out broadcast queue and stop worker.
			n := len(b.queue)

			for i := 0; i < n; i++ {
				item := <-b.queue
				close(item.result)
			}

			return
		default:
		}

		if preferred := b.ledger.Resolver().Preferred(); preferred != nil {
			b.broadcastingNops = false
			b.querying(logger, preferred)
		} else {
			b.gossiping(logger)
		}
	}
}

func (b *broadcaster) querying(logger zerolog.Logger, preferred *wavelet.Transaction) {
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
	default:
		var self common.AccountID
		copy(self[:], b.node.Keys.PublicKey())

		balance, _ := b.ledger.Accounts.ReadAccountBalance(self)

		// If there is nothing we need to broadcast urgently, then broadcast
		// a nop (if we have previously broadcasted a transaction beforehand).
		//
		// If we do not have any balance either, do not broadcast any nops.
		if !b.broadcastingNops || balance < sys.TransactionFeeAmount {
			time.Sleep(10 * time.Millisecond)
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
	if b.ledger.ViewID() == item.tx.ViewID && !b.broadcastingNops {
		b.broadcastingNops = true

		logger.Log().
			Bool("broadcast_nops", true).
			Msg("Started broadcasting nops.")
	}

	if item.result != nil {
		item.result <- nil
	}
}

func (b *broadcaster) query(preferred *wavelet.Transaction) error {
	opcodeQueryResponse, err := noise.OpcodeFromMessage((*QueryResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "broadcast: response opcode not registered")
	}

	peerIDs, err := selectPeers(b.node, sys.SnowballQueryK)
	if err != nil {
		return errors.Wrap(err, "broadcast: cannot query")
	}

	responses, err := broadcast(b.node, peerIDs, QueryRequest{tx: preferred}, opcodeQueryResponse)
	if err != nil {
		return err
	}

	var accountIDs []common.AccountID

	for _, peerID := range peerIDs {
		var accountID common.AccountID
		copy(accountID[:], peerID.PublicKey())

		accountIDs = append(accountIDs, accountID)
	}

	votes := make(map[common.AccountID]common.TransactionID)
	transactions := make(map[common.TransactionID]*wavelet.Transaction)

	for i, res := range responses {
		if res != nil {
			if preferred := res.(QueryResponse).preferred; preferred != nil {
				transactions[preferred.ID] = preferred
				votes[accountIDs[i]] = preferred.ID
			}
		}
	}

	weights := wavelet.ComputeStakeDistribution(b.ledger.Accounts, accountIDs, sys.SnowballQueryK)

	counts := make(map[common.TransactionID]float64)

	for account, preferredID := range votes {
		counts[preferredID] += weights[account]
	}

	if err := b.ledger.ProcessQuery(counts, transactions); err != nil {
		return errors.Wrap(err, "broadcast: failed to process query results")
	}

	return nil
}

func (b *broadcaster) gossip(tx *wavelet.Transaction) error {
	opcodeGossipResponse, err := noise.OpcodeFromMessage((*GossipResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "broadcast: response opcode not registered")
	}

	peerIDs, _ := selectPeers(b.node, sys.SnowballQueryK)
	if len(peerIDs) == 0 {
		return errors.New("broadcast: cannot gossip because not connected to any peers")
	}

	responses, err := broadcast(b.node, peerIDs, GossipRequest{TX: tx}, opcodeGossipResponse)
	if err != nil {
		return err
	}

	var accountIDs []common.AccountID

	for _, peerID := range peerIDs {
		var accountID common.AccountID
		copy(accountID[:], peerID.PublicKey())

		accountIDs = append(accountIDs, accountID)
	}

	votes := make(map[common.AccountID]bool)

	for i, res := range responses {
		if res != nil {
			votes[accountIDs[i]] = res.(GossipResponse).vote
		}
	}

	weights := wavelet.ComputeStakeDistribution(b.ledger.Accounts, accountIDs, sys.SnowballQueryK)

	var accum float64

	for account, vote := range votes {
		if vote {
			accum += weights[account]
		}
	}

	if accum < sys.SnowballQueryAlpha {
		return errors.Errorf("broadcast: less than %.1f%% of queried K peers find tx %x valid", sys.SnowballQueryAlpha*100, tx.ID)
	}

	if err := b.ledger.ReceiveTransaction(tx); err != nil && errors.Cause(err) != wavelet.VoteAccepted {
		return errors.Wrap(err, "broadcast: failed to add successfully queried tx to view-graph")
	}

	return nil
}
