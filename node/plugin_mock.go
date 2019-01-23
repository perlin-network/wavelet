package node

import (
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/wavelet"
)

var _ NodeInterface = (*WaveletMock)(nil)

type WaveletMock struct {
	StartupCallback              func(net *network.Network)
	ReceiveCallback              func(ctx *network.PluginContext) error
	CleanupCallback              func(net *network.Network)
	PeerConnectCallback          func(client *network.PeerClient)
	PeerDisconnectCallback       func(client *network.PeerClient)
	MakeTransactionCallback      func(tag string, payload []byte) *wire.Transaction
	BroadcastTransactionCallback func(wired *wire.Transaction)
	LedgerDoCallback             func(f func(ledger *wavelet.Ledger))
}

func (w *WaveletMock) Startup(net *network.Network) {
	if w.StartupCallback != nil {
		w.StartupCallback(net)
	}
}

func (w *WaveletMock) Receive(ctx *network.PluginContext) error {
	if w.ReceiveCallback != nil {
		return w.ReceiveCallback(ctx)
	}
	return nil
}

func (w *WaveletMock) Cleanup(net *network.Network) {
	if w.CleanupCallback != nil {
		w.CleanupCallback(net)
	}
}

func (w *WaveletMock) PeerConnect(client *network.PeerClient) {
	if w.PeerConnectCallback != nil {
		w.PeerConnectCallback(client)
	}
}

func (w *WaveletMock) PeerDisconnect(client *network.PeerClient) {
	if w.PeerDisconnectCallback != nil {
		w.PeerDisconnectCallback(client)
	}
}

func (w *WaveletMock) MakeTransaction(tag string, payload []byte) *wire.Transaction {
	if w.MakeTransactionCallback != nil {
		return w.MakeTransactionCallback(tag, payload)
	}
	return nil
}

func (w *WaveletMock) BroadcastTransaction(wired *wire.Transaction) {
	if w.BroadcastTransactionCallback != nil {
		w.BroadcastTransactionCallback(wired)
	}
}

func (w *WaveletMock) LedgerDo(f func(ledger *wavelet.Ledger)) {
	if w.LedgerDoCallback != nil {
		w.LedgerDoCallback(f)
	}
}
