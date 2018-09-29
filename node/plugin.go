package node

import (
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/wavelet"
	"github.com/perlin-network/wavelet/log"
)

var _ network.PluginInterface = (*Wavelet)(nil)

type Options struct {
	DatabasePath string
	ServicesPath string
}

type Wavelet struct {
	network.Plugin

	Ledger *wavelet.Ledger
	Wallet *wavelet.Wallet

	opts Options
}

func NewPlugin(opts Options) *Wavelet {
	plugin := &Wavelet{opts: opts}

	return plugin
}

func (w *Wavelet) Startup(net *network.Network) {
	w.Ledger = wavelet.NewLedger(w.opts.DatabasePath, w.opts.ServicesPath)
	w.Ledger.Init()

	w.Wallet = wavelet.NewWallet(net.GetKeys(), w.Ledger.Store)
}

func (w *Wavelet) Receive(ctx *network.PluginContext) error {
	switch msg := ctx.Message().(type) {
	case *wire.Transaction:
		id, successful, err := w.Ledger.RespondToQuery(msg)
		if err != nil {
			log.Warn().Err(err).Msg("Failed to respond to query.")
			return err
		}

		res := &QueryResponse{Id: id, StronglyPreferred: successful}

		err = ctx.Reply(res)
		if err != nil {
			log.Error().Err(err).Msg("Failed to send response.")
			return err
		}
	}
	return nil
}

func (w *Wavelet) Cleanup(net *network.Network) {
	err := w.Ledger.Graph.Cleanup()

	if err != nil {
		panic(err)
	}
}

func (w *Wavelet) PeerConnect(client *network.PeerClient) {

}

func (w *Wavelet) PeerDisconnect(client *network.PeerClient) {

}
