package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/protocol"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/common"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"sync"
	"time"
)

func selectPeers(node *noise.Node, amount int) ([]protocol.ID, error) {
	peerIDs := skademlia.FindClosestPeers(skademlia.Table(node), protocol.NodeID(node).Hash(), amount)

	if len(peerIDs) < sys.SnowballQueryK {
		return peerIDs, errors.Errorf("only connected to %d peer(s), but require a minimum of %d peer(s)", len(peerIDs), sys.SnowballQueryK)
	}

	return peerIDs, nil
}

func broadcast(node *noise.Node, peerIDs []protocol.ID, req noise.Message, resOpcode noise.Opcode) ([]noise.Message, error) {
	var responses []noise.Message

	var mu sync.Mutex
	var wg sync.WaitGroup
	wg.Add(len(peerIDs))

	record := func(account common.AccountID, res noise.Message) {
		mu.Lock()
		defer mu.Unlock()

		responses = append(responses, res)
	}

	for _, peerID := range peerIDs {
		peerID := peerID

		var account common.AccountID
		copy(account[:], peerID.PublicKey())

		go func() {
			defer wg.Done()

			peer := protocol.Peer(node, peerID)
			if peer == nil {
				record(account, nil)
				return
			}

			// Send query request.
			err := peer.SendMessage(req)
			if err != nil {
				record(account, nil)
				return
			}

			select {
			case msg := <-peer.Receive(resOpcode):
				record(account, msg)
			case <-time.After(sys.QueryTimeout):
				record(account, nil)
			}
		}()
	}

	wg.Wait()

	return responses, nil
}
