package node

import (
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/wavelet"
)

var _ NodeInterface = (*WaveletMock)(nil)

type WaveletMock struct {
	StartupCB              func(net *network.Network)
	ReceiveCB              func(ctx *network.PluginContext) error
	CleanupCB              func(net *network.Network)
	PeerConnectCB          func(client *network.PeerClient)
	PeerDisconnectCB       func(client *network.PeerClient)
	MakeTransactionCB      func(tag uint32, payload []byte) *wire.Transaction
	BroadcastTransactionCB func(wired *wire.Transaction)
	LedgerDoCB             func(f func(ledger wavelet.LedgerInterface))
}

func (w *WaveletMock) Startup(net *network.Network) {
	w.StartupCB(net)
}

func (w *WaveletMock) Receive(ctx *network.PluginContext) error {
	return w.ReceiveCB(ctx)
}

func (w *WaveletMock) Cleanup(net *network.Network) {
	w.CleanupCB(net)
}

func (w *WaveletMock) PeerConnect(client *network.PeerClient) {
	w.PeerConnectCB(client)
}

func (w *WaveletMock) PeerDisconnect(client *network.PeerClient) {
	w.PeerDisconnectCB(client)
}

func (w *WaveletMock) MakeTransaction(tag uint32, payload []byte) *wire.Transaction {
	return w.MakeTransactionCB(tag, payload)
}

func (w *WaveletMock) BroadcastTransaction(wired *wire.Transaction) {
	w.BroadcastTransactionCB(wired)
}

func (w *WaveletMock) LedgerDo(f func(ledger wavelet.LedgerInterface)) {
	w.LedgerDoCB(f)
}
