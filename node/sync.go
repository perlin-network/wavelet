package node

import (
	"context"
	"time"

	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/params"
	"github.com/perlin-network/wavelet/stats"

	"github.com/gogo/protobuf/proto"
	"github.com/perlin-network/noise/peer"
)

type syncer struct {
	*Wavelet
}

// randomlySelectPeers randomly selects N closest peers w.r.t. this node.
func (s *syncer) randomlySelectPeers(n int) []peer.ID {
	peers := s.routes.FindClosestPeers(s.net.ID, n+1)

	var ids []peer.ID

	for _, peer := range peers {
		if peer.Address != s.net.ID.Address {
			ids = append(ids, peer)
		}
	}

	return ids
}

// hinterLoop hints transactions we have to other nodes which said nodes may not have.
func (s *syncer) hinterLoop() {
	for {
		time.Sleep(time.Duration(params.SyncHintPeriodMs) * time.Millisecond)

		var tx *wire.Transaction

		s.Ledger.Do(func(l *wavelet.Ledger) {
			if recent := l.Store.GetMostRecentlyUsed(3); len(recent) > 0 {
				// Randomly pick a transaction out of the most recently used.
				symbol := recent[len(recent)/2]

				if len(symbol) > 64 {
					// TODO(kenta): A hack in place as all symbols in tx object cache are prefixed with 'tx_'.
					symbol = symbol[3:]
				}

				raw, err := l.Store.GetBySymbol(symbol)

				if err != nil {
					return
				}

				tx = &wire.Transaction{
					Sender:    raw.Sender,
					Nonce:     raw.Nonce,
					Parents:   raw.Parents,
					Tag:       raw.Tag,
					Payload:   raw.Payload,
					Signature: raw.Signature,
				}

				if tx.Tag == params.TagCreateContract {
					// if it was a create contract that was removed from the db, load the tx payload from the ledger
					if len(tx.Payload) == 0 {
						contractCode, err := l.LoadContract(symbol)
						if err != nil {
							return
						}
						tx.Payload = contractCode
					}
				}
			}
		})

		if tx == nil {
			continue
		}

		s.net.BroadcastRandomly(context.Background(), tx, params.SyncHintNumPeers)

		// update the number of connected peers
		stats.SetNumConnectedPeers(int64(len(s.randomlySelectPeers(1000))))
	}
}

func (s *syncer) Start() {
	go s.hinterLoop()
}

// QueryMissingParents queries other nodes for parents of a children which we may
// not ahve in store.
func (s *syncer) QueryMissingParents(parents []string) {
	pushHint := make([]string, 0)

	s.Ledger.Do(func(l *wavelet.Ledger) {
		for _, p := range parents {
			if !l.Store.TransactionExists(p) {
				pushHint = append(pushHint, p)
			}
		}
	})

	s.net.BroadcastRandomly(context.Background(), &TxPushHint{
		Transactions: pushHint,
	}, params.SyncHintNumPeers)
}

// QueryMissingChildren queries other nodes for children of a transaction which we may
// not have in store.
func (s *syncer) QueryMissingChildren(id string) {
	peers := s.randomlySelectPeers(params.SyncNumPeers)
	children := make(map[string]struct{})

	for _, p := range peers {
		client, err := s.net.Client(p.Address)
		if err != nil {
			log.Error().Err(err).Msg("unable to create client")
			continue
		}

		r, err := func() (proto.Message, error) {
			ctx, cancel := context.WithTimeout(context.Background(), queryTimeout)
			defer cancel()
			msg := &SyncChildrenQueryRequest{
				Id: id,
			}

			return client.Request(ctx, msg)
		}()
		if err != nil {
			log.Error().Err(err).Msg("request failed")
			continue
		}

		res, ok := r.(*SyncChildrenQueryResponse)
		if !ok {
			log.Error().Err(err).Msg("response type mismatch")
			continue
		}

		for _, child := range res.Children {
			children[child] = struct{}{}
		}
	}

	deleteList := make([]string, 0)

	s.Ledger.Do(func(l *wavelet.Ledger) {
		for c := range children {
			if l.Store.TransactionExists(c) {
				deleteList = append(deleteList, c)
			}
		}
	})

	for _, c := range deleteList {
		delete(children, c)
	}

	pushHint := make([]string, 0)
	for c := range children {
		pushHint = append(pushHint, c)
	}

	s.net.BroadcastRandomly(context.Background(), &TxPushHint{
		Transactions: pushHint,
	}, params.SyncHintNumPeers)
}
