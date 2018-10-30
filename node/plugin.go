package node

import (
	"math/rand"
	"os"

	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
	"github.com/perlin-network/wavelet/params"
	"github.com/perlin-network/wavelet/security"

	"github.com/perlin-network/graph/database"
	"github.com/perlin-network/graph/graph"
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/dht"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/noise/network/discovery"
	"github.com/perlin-network/noise/types/opcode"

	"github.com/pkg/errors"
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
	syncer

	net    *network.Network
	routes *dht.RoutingTable

	Ledger *wavelet.LoopHandle
	Wallet *wavelet.Wallet

	opts Options
}

const (
	WireTransactionOpcode           = 4000
	QueryResponseOpcode             = 4010
	SyncChildrenQueryRequestOpcode  = 4011
	SyncChildrenQueryResponseOpcode = 4012
	TxPushHintOpcode                = 4013
)

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

	opcode.RegisterMessageType(opcode.Opcode(WireTransactionOpcode), &wire.Transaction{})
	opcode.RegisterMessageType(opcode.Opcode(QueryResponseOpcode), &QueryResponse{})
	opcode.RegisterMessageType(opcode.Opcode(SyncChildrenQueryRequestOpcode), &SyncChildrenQueryRequest{})
	opcode.RegisterMessageType(opcode.Opcode(SyncChildrenQueryResponseOpcode), &SyncChildrenQueryResponse{})
	opcode.RegisterMessageType(opcode.Opcode(TxPushHintOpcode), &TxPushHint{})

	ledger := wavelet.NewLedger(w.opts.DatabasePath, w.opts.ServicesPath, w.opts.GenesisPath)

	loop := wavelet.NewEventLoop(ledger)
	go loop.RunForever()

	w.Ledger = loop.Handle()

	w.Wallet = wavelet.NewWallet(net.GetKeys())

	w.query = query{Wavelet: w}
	w.query.sybil = stake{query: w.query}

	w.syncer = syncer{Wavelet: w}
	w.syncer.Start()

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

			if rand.Intn(params.SyncNeighborsLikelihood) == 0 {
				go w.syncer.QueryMissingChildren(id)
				go w.syncer.QueryMissingParents(msg.Parents)
			}
		} else {
			go w.syncer.QueryMissingChildren(id)
			go w.syncer.QueryMissingParents(msg.Parents)

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
	case *SyncChildrenQueryRequest:
		var childrenIDs []string

		w.Ledger.Do(func(l *wavelet.Ledger) {
			children, err := l.Store.GetChildrenBySymbol(msg.Id)
			if err != nil {
				log.Error().Err(err).Msg("cannot get children")
			} else {
				childrenIDs = children.Transactions
			}
		})

		if childrenIDs == nil {
			childrenIDs = make([]string, 0)
		}

		ctx.Reply(&SyncChildrenQueryResponse{
			Children: childrenIDs,
		})
	case *TxPushHint:
		for _, id := range msg.Transactions {
			var out *wire.Transaction

			w.Ledger.Do(func(l *wavelet.Ledger) {
				if tx, err := l.Store.GetBySymbol(id); err == nil {
					out = &wire.Transaction{
						Sender:    tx.Sender,
						Nonce:     tx.Nonce,
						Parents:   tx.Parents,
						Tag:       tx.Tag,
						Payload:   tx.Payload,
						Signature: tx.Signature,
					}
				}
			})

			if out != nil {
				w.net.BroadcastByAddresses(out, ctx.Sender().Address)
			}
		}
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
	log.Debug().Interface("ID", client.ID).Msgf("Peer disconnected: %s", client.Address)
}
