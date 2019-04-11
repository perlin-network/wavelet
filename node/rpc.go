package node

import (
	"github.com/perlin-network/noise"
	"github.com/perlin-network/noise/skademlia"
	"github.com/perlin-network/wavelet/sys"
	"github.com/pkg/errors"
	"sync"
	"time"
)

func selectPeers(network *skademlia.Protocol, node *noise.Node, amount int) ([]*noise.Peer, error) {
	peers := network.Peers(node)

	if len(peers) < amount {
		return peers, errors.Errorf("only connected to %d peer(s), but require a minimum of %d peer(s)", len(peers), amount)
	}

	if len(peers) > amount {
		peers = peers[:amount]
	}

	return peers, nil
}

func broadcast(peers []*noise.Peer, reqOpcode byte, resOpcode byte, req []byte) ([][]byte, error) {
	responses := make([][]byte, len(peers))

	var wg sync.WaitGroup
	wg.Add(len(peers))

	for i, peer := range peers {
		i, peer := i, peer

		go func() {
			defer wg.Done()

			mux := peer.Mux()
			defer mux.Close()

			if err := mux.Send(reqOpcode, req); err != nil {
				return
			}

			select {
			case wire := <-mux.Recv(resOpcode):
				responses[i] = wire.Bytes()
			case <-time.After(sys.QueryTimeout):
			}
		}()
	}

	wg.Wait()

	return responses, nil
}
