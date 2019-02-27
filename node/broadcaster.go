package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/signature/eddsa"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"golang.org/x/crypto/blake2b"
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

func (b *broadcaster) Init() {
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
	var zero [wavelet.TransactionIDSize]byte

	for {
		var item broadcastItem

		select {
		case popped := <-b.queue:
			item = popped

			// Start broadcasting nops if we started broadcasting an arbitrary
			// transaction.
			b.broadcastingNops = true
		case <-time.After(1 * time.Millisecond):
			// If there is nothing we need to broadcast urgently, then either broadcast
			// our preferred transaction, or a nop (if we have previously broadcasted
			// a transaction beforehand).

			preferredID := b.ledger.Resolver().Preferred()

			if preferredID == zero {
				if !b.broadcastingNops {
					continue
				}

				nop, err := b.ledger.NewTransaction(b.node.Keys, sys.TagNop, nil)
				if err != nil {
					continue
				}

				item = broadcastItem{tx: nop, result: nil, viewID: b.ledger.ViewID()}
			} else {
				preferred := b.ledger.FindTransaction(preferredID)

				if preferred == nil {
					continue
				}

				item = broadcastItem{tx: preferred, result: nil, viewID: b.ledger.ViewID()}
			}
		}

		if b.ledger.ViewID() != item.viewID {
			err := errors.New("broadcast: consensus round already finalized; submit transaction again")
			err = errors.Wrapf(err, "tx %x", item.tx.ID)

			if item.result != nil {
				item.result <- err
			} else {
				log.Warn().Err(err).Msg("Stopped a broadcast during assertions.")
			}
			continue
		}

		if err := b.query(item.tx); err != nil {
			if item.result != nil {
				item.result <- err
			} else {
				log.Warn().Err(err).Msg("Got an error while querying.")
			}
			continue
		}

		if item.result != nil {
			item.result <- nil
		}

		// If we have advanced one view ID, stop broadcasting nops.
		if b.ledger.ViewID() != item.viewID {
			b.broadcastingNops = false
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

	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(peerIDs))

	responses := make(map[[blake2b.Size256]byte]bool)

	recordResponse := func(rawID []byte, response bool) {
		mu.Lock()
		defer mu.Unlock()

		var id [blake2b.Size256]byte
		copy(id[:], rawID)

		responses[id] = response
	}

	req := QueryRequest{tx: tx}

	for _, peerID := range peerIDs {
		peerID := peerID

		go func() {
			defer wg.Done()

			peer := protocol.Peer(b.node, peerID)
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
			case msg := <-peer.Receive(opcode):
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

	if err = b.ledger.ReceiveTransaction(tx); errors.Cause(err) != wavelet.VoteAccepted {
		return errors.Wrap(err, "broadcast: failed to add successfully queried transaction to view-graph")
	}

	if err = b.ledger.ReceiveQuery(tx, responses); errors.Cause(err) != wavelet.ErrTxNotCritical {
		return errors.Wrap(err, "broadcast: failed to handle critical transaction")
	}

	return nil
}
