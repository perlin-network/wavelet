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
	"sync"
	"time"
)

type broadcastItem struct {
	tx     *wavelet.Transaction
	result chan error

	viewID uint64
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
	item := broadcastItem{tx: tx, result: make(chan error, 1), viewID: b.ledger.ViewID()}
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
		var item broadcastItem

		select {
		case popped := <-b.queue:
			item = popped

			logger.Log().
				Bool("broadcast_nops", b.broadcastingNops).
				Hex("tx_id", popped.tx.ID[:]).
				Msg("Broadcasting out queued transaction.")
		case <-time.After(1 * time.Millisecond):
			// If there is nothing we need to broadcast urgently, then either broadcast
			// our preferred transaction, or a nop (if we have previously broadcasted
			// a transaction beforehand).

			preferredID := b.ledger.Resolver().Preferred()

			if preferredID == common.ZeroTransactionID {
				if !b.broadcastingNops {
					continue
				}

				nop, err := b.ledger.NewTransaction(b.node.Keys, sys.TagNop, nil)
				if err != nil {
					continue
				}

				item = broadcastItem{tx: nop, result: nil, viewID: b.ledger.ViewID()}

				logger.Log().
					Bool("broadcast_nops", true).
					Hex("tx_id", nop.ID[:]).
					Msg("Broadcasting out nop transaction.")
			} else {
				preferred, exists := b.ledger.FindTransaction(preferredID)

				if !exists {
					continue
				}

				item = broadcastItem{tx: preferred, result: nil, viewID: b.ledger.ViewID()}

				logger.Log().
					Bool("broadcast_nops", false).
					Hex("tx_id", preferred.ID[:]).
					Msg("Broadcasting out our preferred transaction.")
			}
		}

		if b.ledger.ViewID() != item.viewID {
			err := errors.New("broadcast: consensus round already finalized; submit transaction again")
			err = errors.Wrapf(err, "tx %x", item.tx.ID)

			if item.result != nil {
				item.result <- err
			} else {
				logger.Warn().
					Err(err).
					Msg("Stopped a broadcast during assertions.")
			}
			continue
		}

		if err := b.query(item.tx); err != nil {
			if item.result != nil {
				item.result <- err
			} else {
				logger.Warn().
					Err(err).
					Msg("Got an error while querying.")
			}
			continue
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
		if b.ledger.ViewID() != item.viewID {
			b.broadcastingNops = false

			logger.Log().
				Bool("broadcast_nops", true).
				Msg("Stopped broadcasting nops.")
		}
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

func (b *broadcaster) query(tx *wavelet.Transaction) error {
	opcode, err := noise.OpcodeFromMessage((*QueryResponse)(nil))
	if err != nil {
		return errors.Wrap(err, "broadcast: query response opcode not registered")
	}

	peerIDs, err := b.selectPeers(sys.SnowballK)
	if err != nil {
		return err
	}

	var accounts []common.AccountID

	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(peerIDs))

	responses := make(map[common.AccountID]common.TransactionID)
	votes := make(map[common.AccountID]bool)

	recordResponse := func(account common.AccountID, preferred common.TransactionID) {
		if preferred != common.ZeroTransactionID {
			responses[account] = preferred
		}
	}

	recordVote := func(account common.AccountID, vote bool) {
		votes[account] = vote
	}

	req := QueryRequest{tx: tx}

	for _, peerID := range peerIDs {
		peerID := peerID

		var account common.AccountID
		copy(account[:], peerID.PublicKey())

		accounts = append(accounts, account)

		go func() {
			defer wg.Done()

			peer := protocol.Peer(b.node, peerID)
			if peer == nil {
				mu.Lock()
				recordVote(account, false)
				mu.Unlock()

				return
			}

			// Send query request.
			err := peer.SendMessage(req)
			if err != nil {
				mu.Lock()
				recordVote(account, false)
				mu.Unlock()

				return
			}

			// Receive query response.
			var res QueryResponse

			select {
			case msg := <-peer.Receive(opcode):
				res = msg.(QueryResponse)
			case <-time.After(sys.QueryTimeout):
				mu.Lock()
				recordVote(account, false)
				mu.Unlock()

				return
			}

			mu.Lock()
			recordVote(account, res.vote)
			recordResponse(account, res.preferred)
			mu.Unlock()
		}()
	}

	wg.Wait()

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

	if err := b.ledger.ProcessQuery(weights, responses); err != nil {
		return errors.Wrap(err, "broadcast: failed to process query results")
	}

	return nil
}
