package node

import (
	"github.com/perlin-network/graph/wire"
	"github.com/perlin-network/noise/network"
	"github.com/perlin-network/wavelet"
)

// NodeInterface provides methods to interact with the network and ledger
type NodeInterface interface {
	Startup(net *network.Network)
	Receive(ctx *network.PluginContext) error
	Cleanup(net *network.Network)
	PeerConnect(client *network.PeerClient)
	PeerDisconnect(client *network.PeerClient)
	MakeTransaction(tag string, payload []byte) *wire.Transaction
	BroadcastTransaction(wired *wire.Transaction)
	LedgerDo(f func(ledger wavelet.LedgerInterface))
}
