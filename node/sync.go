package node

import (
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/graph"
	"github.com/perlin-network/graph/wire"
	rpc2 "github.com/perlin-network/noise/network/rpc"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/security"
	"sync"
	"time"
)

const SyncQueryPeerNum = 10
const SyncChildrenQueryPeerNum = 5
const SyncBroadcastTxNum = 16

type SyncWorker struct {
	wavelet           *Wavelet
	incomingTxIDQueue chan string
	//validTxIDQueue chan string
}

func NewSyncWorker(wavelet *Wavelet) *SyncWorker {
	return &SyncWorker{
		wavelet:           wavelet,
		incomingTxIDQueue: make(chan string, 4096),
		//validTxIDQueue: make(chan string, 4096),
	}
}

// randomlySelectPeers randomly selects N closest peers w.r.t. this node.
func (w *SyncWorker) randomlySelectPeers(n int) ([]string, error) {
	peers := w.wavelet.routes.FindClosestPeers(w.wavelet.net.ID, n+1)

	var addresses []string

	for _, peer := range peers {
		if peer.Address != w.wavelet.net.ID.Address {
			addresses = append(addresses, peer.Address)
		}
	}

	return addresses, nil
}

func (w *SyncWorker) RunSeedBroadcastLoop() {
	for {
		time.Sleep(10 * time.Second)
		var selected []string
		w.wavelet.Ledger.Do(func(l *wavelet.Ledger) {
			selected = l.Graph.Store.GetMostRecentlyUsed(SyncBroadcastTxNum)
		})
		if len(selected) == 0 {
			continue
		}

		peers, err := w.randomlySelectPeers(SyncQueryPeerNum)
		if err != nil {
			log.Error().Err(err).Msg("unable to randomly select broadcast targets")
			continue
		}

		wg := &sync.WaitGroup{}
		for _, p := range peers {
			wg.Add(1)
			go func() {
				defer wg.Done()

				client, err := w.wavelet.net.Client(p)
				if err != nil {
					log.Error().Err(err).Msg("unable to create client")
					return
				}
				client.Tell(&SyncSeed{
					Transactions: selected,
				})
			}()
		}
		wg.Wait()
	}
}

func (w *SyncWorker) AddIncomingTxID(id string) {
	// pre-check to reduce queue pressure.

	var exists bool

	w.wavelet.Ledger.Do(func(l *wavelet.Ledger) {
		exists = l.Graph.Store.TransactionExists(id)
	})

	if !exists {
		select {
		case w.incomingTxIDQueue <- id:
			log.Debug().Msg("tx added to incoming queue")
		default:
			log.Warn().Msg("incoming tx id queue full")
		}
	} else {
		log.Debug().Msg("skipping existing tx")
	}
}

func (w *SyncWorker) Start() {
	go w.RunSeedBroadcastLoop()

	for i := 0; i < 1; i++ {
		go w.RunTxFetchLoop()
	}
}

// concurrent-safe
// Do we need this?
/*
func (w *SyncWorker) RunIDValidationLoop() {
	for txID := range w.incomingTxIDQueue {
		var exists bool

		w.wavelet.Ledger.Do(func(l *wavelet.Ledger) {
			exists = l.Graph.Store.TransactionExists(txID)
		})

		if exists {
			log.Debug().Msg("skipping exist tx in validation loop")
			continue
		}

		peers, err := w.randomlySelectPeers(SyncQueryPeerNum)
		if err != nil {
			log.Warn().Err(err).Msg("failed to select peers")
			continue
		}

		if len(peers) == 0 {
			log.Warn().Msg("no available peers")
			continue
		}

		requiredPositives := (uint64(len(peers)) + 1) / 2
		gotPositives := uint64(0)

		wg := &sync.WaitGroup{}

		for _, p := range peers {
			wg.Add(1)

			go func() {
				defer wg.Done()

				request := new(rpc.Request)
				request.SetTimeout(5 * time.Second)
				request.SetMessage(&SyncTxIDQueryRequest{Id: txID})

				client, err := w.wavelet.net.Client(p)
				if err != nil {
					log.Warn().Err(err).Msg("unable to create client")
					return
				}

				_res, err := client.Request(request)
				if err != nil {
					log.Warn().Err(err).Msg("request failed")
					return
				}

				res, ok := _res.(*SyncTxIDQueryResponse)
				if !ok {
					log.Warn().Msg("invalid response type")
				}

				if res.Positive {
					atomic.AddUint64(&gotPositives, 1)
				}
			}()
		}

		wg.Wait()

		if gotPositives >= requiredPositives {
			w.validTxIDQueue <- txID
			log.Debug().Msg("validation succeeded")
		} else {
			log.Warn().Msg("validation failed")
		}
	}
}
*/

// concurrent-safe
func (w *SyncWorker) RunTxFetchLoop() {
	//for txID := range w.validTxIDQueue {
	for txID := range w.incomingTxIDQueue {
		var exists bool

		w.wavelet.Ledger.Do(func(l *wavelet.Ledger) {
			exists = l.Graph.Store.TransactionExists(txID)
		})

		if exists {
			log.Debug().Msg("skipping existing tx (loop)")
			continue
		}

		peers, err := w.randomlySelectPeers(SyncQueryPeerNum)
		if err != nil {
			log.Warn().Err(err).Msg("failed to select peers")
			continue
		}

		var validatedTx *wire.Transaction

		for _, p := range peers {
			request := new(rpc2.Request)
			request.SetTimeout(5 * time.Second)
			request.SetMessage(&SyncTxFetchRequest{Id: txID})

			client, err := w.wavelet.net.Client(p)
			if err != nil {
				log.Warn().Err(err).Msg("unable to create client")
				continue
			}

			_res, err := client.Request(request)
			if err != nil {
				log.Warn().Err(err).Msg("request failed")
				continue
			}

			tx, ok := _res.(*wire.Transaction)
			if !ok {
				log.Warn().Msg("invalid response type")
				continue
			}

			valid, err := security.ValidateWiredTransaction(tx)
			if err != nil {
				log.Warn().Err(err).Msg("tx validation failed")
				continue
			}

			if !valid {
				log.Warn().Msg("invalid tx")
				continue
			}

			if graph.Symbol(tx) != txID {
				log.Warn().Msg("tx id mismatch")
				continue
			}

			validatedTx = tx
			break
		}

		if validatedTx == nil {
			log.Warn().Msg("cannot fetch and validate tx")
			continue
		}

		id := graph.Symbol(validatedTx)

		successful := false

		w.wavelet.Ledger.Do(func(l *wavelet.Ledger) {
			if l.TransactionExists(id) {
				return
			}

			_, successful, err = l.RespondToQuery(validatedTx)
		})

		if err != nil || !successful {
			continue
		}

		err = w.wavelet.Query(validatedTx)

		if err != nil {
			log.Error().Err(err).Msg("Failed to gossip out transaction which was received.")
			continue
		}

		var tx *database.Transaction

		w.wavelet.Ledger.Do(func(l *wavelet.Ledger) {
			tx, err = l.GetBySymbol(id)
		})

		if err != nil {
			log.Error().Err(err).Msg("Failed to find transaction which was received.")
			continue
		}

		w.wavelet.Ledger.Do(func(l *wavelet.Ledger) {
			err = l.HandleSuccessfulQuery(tx)
		})

		if err != nil {
			log.Error().Err(err).Msg("Failed to update conflict set for transaction received which was gossiped out.")
			continue
		}

		log.Debug().Msg("successfully handled incoming tx")

		w.queryChildrenAsync(id)

		for _, id := range tx.Parents {
			w.AddIncomingTxID(id)
		}
	}
}

func (w *SyncWorker) queryChildrenAsync(id string) {
	peers, err := w.randomlySelectPeers(SyncChildrenQueryPeerNum)
	if err != nil {
		log.Error().Err(err).Msg("unable to select peers")
		return
	}

	for _, p := range peers {
		client, err := w.wavelet.net.Client(p)
		if err != nil {
			log.Error().Err(err).Msg("unable to create client")
			return
		}
		client.Tell(&SyncChildrenQueryHint{
			Id: id,
		})
	}
}
