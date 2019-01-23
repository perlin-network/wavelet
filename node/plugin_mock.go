package node

import (
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/wavelet"
)

var _ NodeInterface = (*WaveletMock)(nil)

type MockOptions struct {
}

type WaveletMock struct {
	opts MockOptions
}

func NewWaveletMock(opts MockOptions) *WaveletMock {
	return &WaveletMock{opts: opts}
}

func (w *WaveletMock) Startup(net *network.Network) {
	return
}

func (w *WaveletMock) Receive(ctx *network.PluginContext) error {
	return nil
}

func (w *WaveletMock) Cleanup(net *network.Network) {
	return
}

func (w *WaveletMock) PeerConnect(client *network.PeerClient) {
	return
}

func (w *WaveletMock) PeerDisconnect(client *network.PeerClient) {
	return
}

func (w *WaveletMock) MakeTransaction(tag string, payload []byte) *wire.Transaction {
	return nil
}

func (w *WaveletMock) BroadcastTransaction(wired *wire.Transaction) {

}

func (w *WaveletMock) LedgerDo(f func(ledger *wavelet.Ledger)) {
}
