package main

import (
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/wavelet/log"
	waveletnode "github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/sys"
	"go.uber.org/atomic"
	"time"
)

func main() {
	log.Register(log.NewConsoleWriter(log.FilterFor(log.ModuleNode, log.ModuleSync, log.ModuleConsensus)))

	serverSent := atomic.NewUint64(0)
	clientRecv := atomic.NewUint64(0)
	serverRecv := atomic.NewUint64(0)

	server := spawnNode(0)
	client := spawnNode(0)

	go func() {
		for range time.Tick(3 * time.Second) {
			fmt.Printf("Server sent %d, client received %d TPS, and server received %d TPS.\n",
				serverSent.Swap(0), clientRecv.Swap(0), serverRecv.Swap(0))
		}
	}()

	server.OnPeerConnected(func(node *noise.Node, peer *noise.Peer) error {
		go func() {
			waveletnode.WaitUntilAuthenticated(peer)

			ledger := waveletnode.Ledger(node)

			opcodeGossipResponse, _ := noise.OpcodeFromMessage((*waveletnode.GossipResponse)(nil))

			for {
				tx, err := ledger.NewTransaction(node.Keys, sys.TagNop, nil)
				if err != nil {
					continue
				}

				_ = peer.SendMessageAsync(&waveletnode.GossipRequest{TX: tx})

				serverSent.Add(1)

				_ = <-peer.Receive(opcodeGossipResponse)

				serverRecv.Add(1)
			}
		}()

		return nil
	})
	_, err := client.Dial(server.ExternalAddress())
	if err != nil {
		panic(err)
	}

	select {}
}
