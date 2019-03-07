package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/cipher/aead"
	"github.com/perlin-network/noise/handshake/ecdh"
	"github.com/perlin-network/noise/log"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"testing"
	"time"
)

func spawn(b *testing.B, peers ...string) *noise.Node {
	params := noise.DefaultParams()
	params.Keys = skademlia.RandomKeys()
	params.MaxMessageSize = 4 * 1024 * 1024
	params.SendMessageTimeout = 1 * time.Second

	n, err := noise.NewNode(params)
	if err != nil {
		b.Fatal(err)
	}

	protocol.New().
		Register(ecdh.New()).
		Register(aead.New()).
		Register(skademlia.New()).
		Register(New()).
		Enforce(n)

	go n.Listen()

	if len(peers) > 0 {
		for _, address := range peers {
			peer, err := n.Dial(address)
			if err != nil {
				b.Fatal(err)
			}

			WaitUntilAuthenticated(peer)
		}

		skademlia.FindNode(n, protocol.NodeID(n).(skademlia.ID), skademlia.BucketSize(), 8)
	}

	return n
}

func BenchmarkLedger(b *testing.B) {
	b.StopTimer()

	log.Disable()

	alice := spawn(b)
	bob := spawn(b, alice.ExternalAddress())

	ledger := Ledger(bob)

	b.StartTimer()
	for i := 0; i < b.N; i++ {
		tx, err := ledger.NewTransaction(bob.Keys, sys.TagNop, nil)
		if err != nil {
			continue
		}

		err = BroadcastTransaction(bob, tx)
		if err != nil {
			continue
		}
	}
	b.StopTimer()
}
