package node

import (
	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/graph"
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/dht"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/noise/network/discovery"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/security"
	"github.com/pkg/errors"
)

var _ network.PluginInterface = (*Wavelet)(nil)
var PluginID = (*Wavelet)(nil)

type Options struct {
	DatabasePath string
	ServicesPath string
	GenesisCSV   string
}

type Wavelet struct {
	query
	syncer
	broadcaster

	net    *network.Network
	routes *dht.RoutingTable

	Ledger *wavelet.LoopHandle
	Wallet *wavelet.Wallet

	opts Options
}

func NewPlugin(opts Options) *Wavelet {
	return &Wavelet{opts: opts}
}

func (w *Wavelet) Startup(net *network.Network) {
	w.net = net

	plugin, registered := net.Plugin(discovery.PluginID)

	if !registered {
		log.Fatal().Msg("net was not built with peer discovery plugin")
	}

	w.routes = plugin.(*discovery.Plugin).Routes

	ledger := wavelet.NewLedger(w.opts.DatabasePath, w.opts.ServicesPath, w.opts.GenesisCSV)

	loop := wavelet.NewEventLoop(ledger)
	go loop.RunForever()

	w.Ledger = loop.Handle()

	w.Wallet = wavelet.NewWallet(net.GetKeys())

	w.query = query{Wavelet: w}
	w.query.sybil = stake{query: w.query}

	w.syncer = syncer{Wavelet: w, kill: make(chan struct{}, 1)}
	go w.syncer.Init()

	w.broadcaster = broadcaster{Wavelet: w}
}

func (w *Wavelet) Receive(ctx *network.PluginContext) error {
	switch msg := ctx.Message().(type) {
	case *wire.Transaction:
		if validated, err := security.ValidateWiredTransaction(msg); err != nil || !validated {
			return errors.Wrap(err, "failed to validate incoming tx")
		}

		id := graph.Symbol(msg)

		var existed bool

		w.Ledger.Do(func(l *wavelet.Ledger) {
			existed = l.TransactionExists(id)
		})

		if existed {
			var successful bool

			w.Ledger.Do(func(l *wavelet.Ledger) {
				successful = l.IsStronglyPreferred(id)

				if successful && !l.WasAccepted(id) {
					err := l.QueueForAcceptance(id)

					if err != nil {
						log.Error().Err(err).Msg("Failed to queue transaction to pend for acceptance.")
					}
				}
			})

			log.Debug().Str("id", id).Str("tag", msg.Tag).Msgf("Received an existing transaction, and voted '%t' for it.", successful)

			res := &QueryResponse{Id: id, StronglyPreferred: successful}

			err := ctx.Reply(res)
			if err != nil {
				log.Error().Err(err).Msg("Failed to send response.")
				return err
			}
		} else {
			var successful bool
			var err error
			w.Ledger.Do(func(l *wavelet.Ledger) {
				_, successful, err = l.RespondToQuery(msg)
				if err == nil && successful {
					err = l.QueueForAcceptance(id)
				}
			})
			if err != nil {
				log.Warn().Err(err).Msg("Failed to respond to query or queue transaction to pend for acceptance")
				return err
			}

			log.Debug().Str("id", id).Str("tag", msg.Tag).Msgf("Received a new transaction, and voted '%t' for it.", successful)

			res := &QueryResponse{Id: id, StronglyPreferred: successful}

			err = ctx.Reply(res)
			if err != nil {
				log.Error().Err(err).Msg("Failed to send response.")
				return err
			}

			go func() {
				err := w.Query(msg)

				if err != nil {
					log.Error().Err(err).Msg("Failed to gossip out transaction which was received.")
					return
				}

				var tx *database.Transaction
				w.Ledger.Do(func(l *wavelet.Ledger) {
					tx, err = l.GetBySymbol(id)
					if err == nil {
						err = l.HandleSuccessfulQuery(tx)
					}
				})
				if err != nil {
					log.Error().Err(err).Msg("Failed to find transaction which was received or update conflict set for transaction received which was gossiped out.")
				}
			}()
		}
	case *SyncRequest:
		err := ctx.Reply(w.RespondToSync(msg))

		if err != nil {
			log.Error().Err(err).Msg("Failed to send response.")
			return err
		}

	}
	return nil
}

func (w *Wavelet) Cleanup(net *network.Network) {
	w.syncer.kill <- struct{}{}

	w.Ledger.Do(func(l *wavelet.Ledger) {
		err := l.Graph.Cleanup()

		if err != nil {
			panic(err)
		}
	})
}

func (w *Wavelet) PeerConnect(client *network.PeerClient) {

}

func (w *Wavelet) PeerDisconnect(client *network.PeerClient) {

}
