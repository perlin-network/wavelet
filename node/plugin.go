package node

import (
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
}

type Wavelet struct {
	query
	syncer
	broadcaster

	net    *network.Network
	routes *dht.RoutingTable

	Ledger *wavelet.Ledger
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

	w.Ledger = wavelet.NewLedger(w.opts.DatabasePath, w.opts.ServicesPath)
	w.Ledger.Init()

	w.Wallet = wavelet.NewWallet(net.GetKeys(), w.Ledger.Store)

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
		existed := w.Ledger.TransactionExists(id)

		if existed {
			successful := w.Ledger.IsStronglyPreferred(id)

			if successful && !w.Ledger.WasAccepted(id) {
				err := w.Ledger.QueueForAcceptance(id)

				if err != nil {
					log.Error().Err(err).Msg("Failed to queue transaction to pend for acceptance.")
				}
			}

			log.Debug().Str("id", id).Interface("tx", msg).Msgf("Received a transaction, and voted '%t' for it.", successful)

			res := &QueryResponse{Id: id, StronglyPreferred: successful}

			err := ctx.Reply(res)
			if err != nil {
				log.Error().Err(err).Msg("Failed to send response.")
				return err
			}
		} else {
			_, successful, err := w.Ledger.RespondToQuery(msg)
			if err != nil {
				log.Warn().Err(err).Msg("Failed to respond to query.")
				return err
			}

			if successful {
				err = w.Ledger.QueueForAcceptance(id)

				if err != nil {
					log.Error().Err(err).Msg("Failed to queue transaction to pend for acceptance.")
				}
			}

			log.Debug().Str("id", id).Interface("tx", msg).Msgf("Received a transaction, and voted '%t' for it.", successful)

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

				tx, err := w.Ledger.GetBySymbol(id)
				if err != nil {
					log.Error().Err(err).Msg("Failed to find transaction which was received.")
					return
				}

				err = w.Ledger.HandleSuccessfulQuery(tx)
				if err != nil {
					log.Error().Err(err).Msg("Failed to update conflict set for transaction received which was gossiped out.")
					return
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

	err := w.Ledger.Graph.Cleanup()

	if err != nil {
		panic(err)
	}
}

func (w *Wavelet) PeerConnect(client *network.PeerClient) {

}

func (w *Wavelet) PeerDisconnect(client *network.PeerClient) {

}
