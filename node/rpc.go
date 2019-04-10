package node

import (
	"fmt"
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
	responseChan := make(chan []byte, len(peers))
	defer close(responseChan)

	var wg sync.WaitGroup
	wg.Add(len(peers))

	for _, peer := range peers {
		go func(peer *noise.Peer) {
			mux := peer.Mux()
			defer func() {
				_ = mux.Close()
				wg.Done()
			}()

			if err := mux.Send(reqOpcode, req); err != nil {
				fmt.Println("Error on peer mux send", err)
				return
			}

			select {
			case wire := <-mux.Recv(resOpcode):
				responseChan <- wire.Bytes()
			case <-time.After(sys.QueryTimeout):
				fmt.Println("Timeout on reading from peer after broadcast")
			}
		}(peer)
	}

	wg.Wait()

	responses := make([][]byte, len(responseChan))
	for i:= range responses {
		responses[i] = <-responseChan
	}

	return responses, nil
}
