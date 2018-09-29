package node

import (
	"fmt"
	"github.com/perlin-network/graph/graph"
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/network/rpc"
	"github.com/perlin-network/wavelet/iblt"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/security"
	"sync"
	"sync/atomic"
	"time"
)

type syncer struct {
	*Wavelet

	kill chan struct{}
}

func (s *syncer) RespondToSync(req *SyncRequest) *SyncResponse {
	res := new(SyncResponse)

	peerTable := iblt.New(iblt.M, iblt.K, iblt.KeySize, iblt.ValueSize, iblt.HashKeySize, nil)
	if req.Table != nil {
		peerTable.UnmarshalProto(req.Table)
	}

	missing := ibltKeys(peerTable.Diff(s.Ledger.IBLT))

	fmt.Println(missing)

	for _, id := range missing {
		tx, err := s.Ledger.GetBySymbol(id)

		if err != nil {
			continue
		}

		wired := &wire.Transaction{
			Sender:    tx.Sender,
			Nonce:     tx.Nonce,
			Parents:   tx.Parents,
			Tag:       tx.Tag,
			Payload:   tx.Payload,
			Signature: tx.Signature,
		}

		res.Transactions = append(res.Transactions, wired)
	}

	return res
}

func (s *syncer) Init() {
	timer := time.NewTicker(1000 * time.Millisecond)

	for {
		select {
		case <-s.kill:
			break
		case <-timer.C:
			s.sync()
		}
	}

	timer.Stop()
}

func (s *syncer) sync() {
	peers, err := s.randomlySelectPeers(2)
	if err != nil {
		return
	}

	if len(peers) == 0 {
		return
	}

	var wg sync.WaitGroup
	wg.Add(len(peers))

	request := new(rpc.Request)
	request.SetTimeout(5 * time.Second)
	request.SetMessage(&SyncRequest{Table: s.Ledger.IBLT.MarshalProto()})

	var received []*wire.Transaction
	var mutex sync.Mutex

	for _, peer := range peers {
		go func(address string) {
			defer wg.Done()

			client, err := s.net.Client(address)
			if err != nil {
				return
			}

			r, err := client.Request(request)
			if err != nil {
				return
			}

			res, ok := r.(*SyncResponse)
			if !ok {
				return
			}

			mutex.Lock()
			received = append(received, res.Transactions...)
			mutex.Unlock()
		}(peer)
	}

	wg.Wait()

	total := uint64(0)

	for _, wired := range received {
		if validated, err := security.ValidateWiredTransaction(wired); err != nil || !validated {
			continue
		}

		id := graph.Symbol(wired)

		if s.Ledger.TransactionExists(id) {
			continue
		}

		_, successful, err := s.Ledger.RespondToQuery(wired)
		if err != nil {
			continue
		}

		if successful {
			err = s.Ledger.QueueForAcceptance(id)

			if err != nil {
				continue
			}
		}

		go func(wired *wire.Transaction) {
			err := s.Query(wired)

			if err != nil {
				log.Error().Err(err).Msg("Failed to gossip out transaction which was received.")
				return
			}

			tx, err := s.Ledger.GetBySymbol(id)
			if err != nil {
				log.Error().Err(err).Msg("Failed to find transaction which was received.")
				return
			}

			err = s.Ledger.HandleSuccessfulQuery(tx)
			if err != nil {
				log.Error().Err(err).Msg("Failed to update conflict set for transaction received which was gossiped out.")
				return
			}

			atomic.AddUint64(&total, 1)
		}(wired)
	}

	if count := atomic.LoadUint64(&total); count > 0 {
		log.Info().
			Uint64("num_synced", count).
			Int("num_peers", len(peers)).
			Msg("Synchronized transactions.")
	}
}

// randomlySelectPeers randomly selects N closest peers w.r.t. this node.
func (s *syncer) randomlySelectPeers(n int) ([]string, error) {
	peers := s.routes.FindClosestPeers(s.net.ID, n+1)

	var addresses []string

	for _, peer := range peers {
		if peer.Address != s.net.ID.Address {
			addresses = append(addresses, peer.Address)
		}
	}

	return addresses, nil
}

// ibltKeys returns the string keys of an IBLT table.
func ibltKeys(table *iblt.Table) (keys []string) {
	if table != nil && !table.IsEmpty() {
		ids := table.List()

		for _, pair := range ids {
			keys = append(keys, pair[0])
		}
	}

	return keys
}
