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
	"os"
)

var _ network.PluginInterface = (*Wavelet)(nil)
var PluginID = (*Wavelet)(nil)

type Options struct {
	DatabasePath  string
	ServicesPath  string
	GenesisPath   string
	ResetDatabase bool
}

type Wavelet struct {
	query
	broadcaster

	syncWorker *SyncWorker

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
		log.Fatal().Msg("Wavelet requires `discovery.Plugin` from the `noise` lib. to be registered into this nodes network.")
	}

	w.routes = plugin.(*discovery.Plugin).Routes

	if w.opts.ResetDatabase {
		err := os.RemoveAll(w.opts.DatabasePath)

		if err != nil {
			log.Info().Err(err).Str("db_path", w.opts.DatabasePath).Msg("Failed to delete previous database instance.")
		} else {
			log.Info().Str("db_path", w.opts.DatabasePath).Msg("Deleted previous database instance.")
		}
	}

	ledger := wavelet.NewLedger(w.opts.DatabasePath, w.opts.ServicesPath, w.opts.GenesisPath)

	loop := wavelet.NewEventLoop(ledger)
	go loop.RunForever()

	w.Ledger = loop.Handle()

	w.Wallet = wavelet.NewWallet(net.GetKeys())

	w.query = query{Wavelet: w}
	w.query.sybil = stake{query: w.query}

	w.syncWorker = NewSyncWorker(w)
	w.syncWorker.Start()

	w.broadcaster = broadcaster{Wavelet: w}
}

func (w *Wavelet) Receive(ctx *network.PluginContext) error {
	switch msg := ctx.Message().(type) {
	case *wire.Transaction:
		if validated, err := security.ValidateWiredTransaction(msg); err != nil || !validated {
			return errors.Wrap(err, "failed to validate incoming tx")
		}

		id := graph.Symbol(msg)

		res := &QueryResponse{Id: id}

		defer func() {
			err := ctx.Reply(res)
			if err != nil {
				log.Error().Err(err).Msg("Failed to send response.")
			}
		}()

		var existed bool

		w.Ledger.Do(func(l *wavelet.Ledger) {
			existed = l.TransactionExists(id)
		})

		if existed {
			w.Ledger.Do(func(l *wavelet.Ledger) {
				res.StronglyPreferred = l.IsStronglyPreferred(id)

				if res.StronglyPreferred && !l.WasAccepted(id) {
					err := l.QueueForAcceptance(id)

					if err != nil {
						log.Error().Err(err).Msg("Failed to queue transaction to pend for acceptance.")
					}
				}
			})

			log.Debug().Str("id", id).Str("tag", msg.Tag).Msgf("Received an existing transaction, and voted '%t' for it.", res.StronglyPreferred)
		} else {
			var err error

			w.Ledger.Do(func(l *wavelet.Ledger) {
				_, res.StronglyPreferred, err = l.RespondToQuery(msg)

				if err == nil && res.StronglyPreferred {
					err = l.QueueForAcceptance(id)
				}
			})

			if err != nil {
				if errors.Cause(err) == database.ErrTxExists {
					return nil
				}

				log.Warn().Err(err).Msg("Failed to respond to query or queue transaction to pend for acceptance.")
				return err
			}

			log.Debug().Str("id", id).Str("tag", msg.Tag).Msgf("Received a new transaction, and voted '%t' for it.", res.StronglyPreferred)

			go func() {
				err := w.Query(msg)

				if err != nil {
					log.Error().Err(err).Msg("Failed to gossip out transaction which was received.")
					return
				}

				var tx *database.Transaction

				w.Ledger.Do(func(l *wavelet.Ledger) {
					tx, err = l.GetBySymbol(id)
				})

				if err != nil {
					log.Error().Err(err).Msg("Failed to find transaction which was received which was gossiped out.")
					return
				}

				w.Ledger.Do(func(l *wavelet.Ledger) {
					err = l.HandleSuccessfulQuery(tx)
				})

				if err != nil {
					log.Error().Err(err).Msg("Failed to update conflict set for transaction received which was gossiped out.")
				}
			}()
		}
	case *SyncSeed:
		for _, id := range msg.Transactions {
			w.syncWorker.AddIncomingTxID(id)
		}
	case *SyncChildrenQueryHint:
		var children []string

		w.Ledger.Do(func(l *wavelet.Ledger) {
			_children, err := l.Graph.Store.GetChildrenBySymbol(msg.Id)
			if err == nil {
				children = _children.Transactions
			} else {
				log.Warn().Err(err).Msg("cannot get children")
			}
		})

		if len(children) == 0 {
			log.Debug().Msg("no children")
			return nil
		}

		client, err := w.net.Client(ctx.Sender().Address)
		if err != nil {
			log.Warn().Err(err).Msg("cannot create client")
			return nil
		}

		client.Tell(&SyncSeed{
			Transactions: children,
		})
		log.Debug().Msgf("sent %d children", len(children))
	}
	return nil
}

func (w *Wavelet) Cleanup(net *network.Network) {
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
