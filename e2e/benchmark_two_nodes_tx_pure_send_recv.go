package main

import (
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/wavelet"
	waveletnode "github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/sys"
	"go.uber.org/atomic"
	"time"
)

func main() {
	serverSent := atomic.NewUint64(0)
	clientRecv := atomic.NewUint64(0)

	opcodeTransaction := noise.RegisterMessage(noise.NextAvailableOpcode(), (*wavelet.Transaction)(nil))

	server := spawnNode(0)
	client := spawnNode(0)

	go func() {
		for range time.Tick(3 * time.Second) {
			sent, received := serverSent.Swap(0), clientRecv.Swap(0)

			fmt.Printf("Sent %d, and received %d TPS.\n", sent, received)
		}
	}()

	server.OnPeerConnected(func(node *noise.Node, peer *noise.Peer) error {
		go func() {
			waveletnode.WaitUntilAuthenticated(peer)

			ledger := waveletnode.Ledger(node)

			for {
				tx, err := ledger.NewTransaction(node.Keys, sys.TagNop, nil)
				if err != nil {
					continue
				}

				_ = peer.SendMessageAsync(tx)

				serverSent.Add(1)
			}
		}()

		return nil
	})

	client.OnPeerDialed(func(node *noise.Node, peer *noise.Peer) error {
		go func() {
			for {
				<-peer.Receive(opcodeTransaction)

				clientRecv.Add(1)
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
