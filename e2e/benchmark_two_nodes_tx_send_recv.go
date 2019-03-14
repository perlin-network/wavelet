package main

import (
	"fmt"
	"github.com/perlin-network/noise"
	"github.com/perlin-network/wavelet"
	waveletnode "github.com/perlin-network/wavelet/node"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"go.uber.org/atomic"
	"time"
)

func main() {
	serverSent := atomic.NewUint64(0)
	clientRecv := atomic.NewUint64(0)
	serverRecv := atomic.NewUint64(0)

	opcodeTransaction := noise.RegisterMessage(noise.NextAvailableOpcode(), (*wavelet.Transaction)(nil))

	server := spawnNode(0)
	client := spawnNode(0)

	go func() {
		for range time.Tick(1 * time.Second) {
			fmt.Printf("Server sent %d, client received %d TPS, and server received %d TPS.\n",
				serverSent.Swap(0), clientRecv.Swap(0), serverRecv.Swap(0))
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

				if err = <-peer.SendMessageAsync(tx); err != nil {
					fmt.Println(err)
				}

				serverSent.Add(1)
			}
		}()

		go func() {
			waveletnode.WaitUntilAuthenticated(peer)

			for {
				_ = <-peer.Receive(opcodeTransaction)
				serverRecv.Add(1)
			}
		}()

		return nil
	})

	client.OnPeerDialed(func(node *noise.Node, peer *noise.Peer) error {
		go func() {
			waveletnode.WaitUntilAuthenticated(peer)

			ledger := waveletnode.Ledger(node)

			for {
				msg := <-peer.Receive(opcodeTransaction)
				tx := msg.(wavelet.Transaction)

				if err := ledger.ReceiveTransaction(&tx); errors.Cause(err) != wavelet.VoteAccepted {
					fmt.Println(err)
				}

				clientRecv.Add(1)

				_ = peer.SendMessageAsync(tx)
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
