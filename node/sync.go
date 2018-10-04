package node

import (
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/graph"
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/network/rpc"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/params"
	"github.com/perlin-network/wavelet/security"
	"github.com/sasha-s/go-IBLT"
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

	peerTable := iblt.New(params.TxK, params.TxL)
	if req.Table != nil {
		err := peerTable.UnmarshalBinary(req.Table)

		if err != nil {
			log.Warn().Err(err).Msg("Failed to unmarshal peers transaction IBLT.")
			return res
		}
	}

	var selfTable *iblt.Filter
	var diff *iblt.Diff
	var err error

	s.Ledger.Do(func(l *wavelet.Ledger) {
		selfTable = l.IBLT.Clone()
	})

	err = selfTable.Sub(*peerTable)

	if err != nil {
		log.Warn().Err(err).Msg("Failed to diff() our IBLT w.r.t. our peers transaction IBLT.")
		return res
	}

	diff, err = selfTable.Decode()
	if err != nil {
		log.Warn().Err(err).Msg("Failed to decode the diff. of our IBLT w.r.t. our peers transaction IBLT.")
	}

	var tx *database.Transaction

	for _, id := range diff.Added {
		s.Ledger.Do(func(l *wavelet.Ledger) {
			tx, err = l.GetBySymbol(string(id))
		})

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

		if len(res.Transactions) > 100 {
			break
		}
	}

	return res
}

func (s *syncer) Init() {
	for {
		select {
		case <-s.kill:
			break
		default:
		}

		s.sync()

		time.Sleep(params.SyncPeriod)
	}
}

func (s *syncer) sync() {
	peers, err := s.randomlySelectPeers(params.SyncNumPeers)
	if err != nil {
		return
	}

	if len(peers) == 0 {
		return
	}

	wg := new(sync.WaitGroup)
	wg.Add(len(peers))

	var encoded []byte

	s.Ledger.Do(func(l *wavelet.Ledger) {
		encoded, err = l.IBLT.MarshalBinary()
	})

	if err != nil {
		return
	}

	request := new(rpc.Request)
	request.SetTimeout(5 * time.Second)
	request.SetMessage(&SyncRequest{Table: encoded})

	var received []*wire.Transaction
	var mutex sync.Mutex

	for _, peer := range peers {
		go func(address string) {
			defer wg.Done()

			client, err := s.net.Client(address)
			if err != nil {
				log.Error().Err(err).Msg("")
				return
			}

			r, err := client.Request(request)
			if err != nil {
				log.Error().Err(err).Msg("")
				return
			}

			res, ok := r.(*SyncResponse)
			if !ok {
				log.Error().Msg("node: response could not be converted to SyncResponse")
				return
			}

			mutex.Lock()
			received = append(received, res.Transactions...)
			mutex.Unlock()
		}(peer)
	}

	wg.Wait()

	wg = new(sync.WaitGroup)

	total := uint64(0)

	for _, wired := range received {
		if validated, err := security.ValidateWiredTransaction(wired); err != nil || !validated {
			continue
		}

		id := graph.Symbol(wired)

		successful := false

		s.Ledger.Do(func(l *wavelet.Ledger) {
			if l.TransactionExists(id) {
				return
			}

			_, successful, err = l.RespondToQuery(wired)
		})

		if err != nil || !successful {
			continue
		}

		wg.Add(1)

		go func(wired *wire.Transaction) {
			defer wg.Done()

			err := s.Query(wired)

			if err != nil {
				log.Error().Err(err).Msg("Failed to gossip out transaction which was received.")
				return
			}

			var tx *database.Transaction

			s.Ledger.Do(func(l *wavelet.Ledger) {
				tx, err = l.GetBySymbol(id)
			})

			if err != nil {
				log.Error().Err(err).Msg("Failed to find transaction which was received.")
				return
			}

			s.Ledger.Do(func(l *wavelet.Ledger) {
				err = l.HandleSuccessfulQuery(tx)
			})

			if err != nil {
				log.Error().Err(err).Msg("Failed to update conflict set for transaction received which was gossiped out.")
				return
			}

			atomic.AddUint64(&total, 1)
		}(wired)
	}

	wg.Wait()

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
